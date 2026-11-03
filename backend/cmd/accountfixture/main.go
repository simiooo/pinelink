// accountfixture runs the real account/API stack against an isolated test
// schema. Its loopback-only mail probe is never part of the server image.
package main

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
	"github.com/simiooo/pinelink/backend/internal/accounts"
	"github.com/simiooo/pinelink/backend/internal/httpapi"
	"github.com/simiooo/pinelink/backend/internal/service"
)

type mailTransport struct {
	mu   sync.Mutex
	mail map[string]any
}

func (m *mailTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.URL.Host != "api.resend.com" || r.URL.Path != "/emails" {
		return nil, fmt.Errorf("fixture denies external mail")
	}
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		return nil, err
	}
	m.mu.Lock()
	m.mail = payload
	m.mu.Unlock()
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(`{"id":"` + uuid.NewString() + `"}`))}, nil
}
func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "account fixture failed:", err)
		os.Exit(1)
	}
}
func run() error {
	dsn := os.Getenv("PINELINK_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("isolated test database required")
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return err
	}
	defer admin.Close()
	schema := "auth_browser_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		return err
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = admin.Exec(cleanup, `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
	}()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return err
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	sqlDB := stdlib.OpenDB(*config.ConnConfig)
	defer sqlDB.Close()
	dir, _ := filepath.Abs("../infra/migrations")
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, os.DirFS(dir))
	if err != nil {
		return err
	}
	if _, err = provider.Up(ctx); err != nil {
		return err
	}
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		return err
	}
	defer pool.Close()
	mail := &mailTransport{}
	blobs := &fixtureBlobs{data: map[string][]byte{}}
	svc := service.New(pool, blobs)
	if err = svc.ConfigureRelayTransport("websocket"); err != nil {
		return err
	}
	if err = svc.SetReplicationKey(ed25519.NewKeyFromSeed(make([]byte, 32))); err != nil {
		return err
	}
	if err = svc.ConfigureAccounts(accounts.Config{Required: true, GuestsEnabled: true, Origin: "http://localhost:5184", EncryptionKey: base64.StdEncoding.EncodeToString(make([]byte, 32)), ResendKey: "fixture-only", MailFrom: "Pinelink <no-reply@example.com>", MailClient: &http.Client{Transport: mail}, GoogleClientID: "fixture-client", GoogleClientSecret: "fixture-secret", GoogleRedirectURI: "http://localhost:5184"}); err != nil {
		return err
	}
	api := httpapi.NewServer("127.0.0.1:8084", svc, log.NewStdLogger(io.Discard), true, []string{"http://localhost:5184"})
	probeMux := http.NewServeMux()
	probeMux.HandleFunc("GET /mail", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		mail.mu.Lock()
		defer mail.mu.Unlock()
		_ = json.NewEncoder(w).Encode(mail.mail)
	})
	probeMux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) {
		v := map[string]int64{}
		for _, table := range []string{"accounts", "guest_identities", "guest_sessions", "guest_pair_grants", "transfer_requests", "delivery_receipts"} {
			var n int64
			if e := pool.QueryRow(r.Context(), `SELECT count(*) FROM `+table).Scan(&n); e != nil {
				http.Error(w, "fixture query failed", 500)
				return
			}
			v[table] = n
		}
		_ = json.NewEncoder(w).Encode(v)
	})
	probeMux.HandleFunc("POST /delay", func(w http.ResponseWriter, r *http.Request) {
		n, _ := strconv.ParseInt(r.URL.Query().Get("ms"), 10, 64)
		if n < 0 || n > 250 {
			http.Error(w, "invalid test delay", 400)
			return
		}
		blobs.delay.Store(n)
		w.WriteHeader(204)
	})
	for path, table := range map[string]string{"POST /expire-invite": "guest_invites", "POST /expire-transfer": "transfer_requests"} {
		probeMux.HandleFunc(path, func(w http.ResponseWriter, r *http.Request) {
			if _, e := uuid.Parse(r.URL.Query().Get("id")); e != nil {
				http.Error(w, "invalid test id", 400)
				return
			}
			_, e := pool.Exec(r.Context(), `UPDATE `+table+` SET expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, r.URL.Query().Get("id"))
			if e != nil {
				http.Error(w, "fixture query failed", 500)
				return
			}
			w.WriteHeader(204)
		})
	}
	probe := &http.Server{Addr: "127.0.0.1:8085", Handler: probeMux, ReadHeaderTimeout: 3 * time.Second}
	go probe.ListenAndServe()
	go svc.RunAccountMail(ctx)
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = probe.Shutdown(shutdown)
		_ = api.Stop(shutdown)
	}()
	return api.Start(ctx)
}
