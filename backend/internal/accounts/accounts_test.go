package accounts

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type fakeSender struct {
	count atomic.Int32
	gate  func()
}

func (f *fakeSender) Send(ctx context.Context, id string, m Mail) (string, error) {
	f.count.Add(1)
	if f.gate != nil {
		f.gate()
	}
	return id, nil
}
func testService(t *testing.T) *Service {
	t.Helper()
	dsn := os.Getenv("PINELINK_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "account_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	sqlDB := stdlib.OpenDB(*cfg.ConnConfig)
	dir, _ := filepath.Abs("../../../infra/migrations")
	migrations, err := goose.NewProvider(goose.DialectPostgres, sqlDB, os.DirFS(dir))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = migrations.Up(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		sqlDB.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = admin.Exec(ctx, `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
		admin.Close()
	})
	s, err := New(pool, Config{Required: true, Origin: "https://pinelink.example", EncryptionKey: base64.StdEncoding.EncodeToString(make([]byte, 32))})
	if err != nil {
		t.Fatal(err)
	}
	s.sender = &fakeSender{}
	return s
}
func requestFor(t *testing.T, s *Service, kind string) Request {
	t.Helper()
	pub, _, _ := ed25519.GenerateKey(rand.Reader)
	r, err := s.Create(t.Context(), CreateRequest{ProofKey: base64.StdEncoding.EncodeToString(pub), Challenge: base64.RawURLEncoding.EncodeToString(hash(randomToken(32))), Kind: kind}, "test-browser")
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func messageFor(t *testing.T, s *Service, c EmailChallenge) Mail {
	t.Helper()
	var encrypted []byte
	if err := s.db.QueryRow(t.Context(), `SELECT payload_cipher FROM account_mail_outbox WHERE challenge_id=$1 AND generation=$2`, c.ID, c.Generation).Scan(&encrypted); err != nil {
		t.Fatal(err)
	}
	plain, err := s.open(encrypted)
	if err != nil {
		t.Fatal(err)
	}
	var m Mail
	if err = json.Unmarshal(plain, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

const testPassword = "a violet otter carries 47 stones"

func signup(t *testing.T, s *Service, email string) (Request, EmailChallenge) {
	t.Helper()
	r := requestFor(t, s, "web")
	c, err := s.StartEmail(t.Context(), r, "signup", email, testPassword)
	if err != nil {
		t.Fatal(err)
	}
	m := messageFor(t, s, c)
	v, err := s.VerifyEmail(t.Context(), r, VerifyInput{ChallengeID: c.ID, Generation: 1, Code: m.Code})
	if err != nil || v.State != "ready" {
		t.Fatalf("signup: state=%s error=%v", v.State, err)
	}
	r, err = s.Request(t.Context(), r.ID)
	if err != nil {
		t.Fatal(err)
	}
	return r, c
}
func claimInput(t *testing.T, r Request) ClaimInput {
	t.Helper()
	key, private, _ := ed25519.GenerateKey(rand.Reader)
	ageKey, _ := age.GenerateX25519Identity()
	in := ClaimInput{DeviceID: "test-" + uuid.NewString(), SigningKey: base64.StdEncoding.EncodeToString(key), EncryptionKey: ageKey.Recipient().String()}
	payload := "pinelink-enrollment-v1\n" + r.ID + "\n" + *r.AccountID + "\n" + in.DeviceID + "\n" + in.SigningKey + "\n" + in.EncryptionKey
	in.DeviceSignature = base64.StdEncoding.EncodeToString(ed25519.Sign(private, []byte(payload)))
	return in
}
func TestSignupProofAndClaimSerialize(t *testing.T) {
	s := testService(t)
	r := requestFor(t, s, "web")
	c, err := s.StartEmail(t.Context(), r, "signup", "Alice@example.com", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	m := messageFor(t, s, c)
	token := strings.TrimPrefix(m.Link, s.Origin()+"/a/")
	if len(token) != 22 {
		t.Fatal("short link length")
	}
	for i := 0; i < 3; i++ {
		if _, err = s.InspectLink(t.Context(), token); err != nil {
			t.Fatal("inspection consumed proof")
		}
	}
	var won atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Go(func() {
			v, e := s.VerifyEmail(t.Context(), r, VerifyInput{ChallengeID: c.ID, Generation: 1, Code: m.Code})
			if e == nil && v.State == "ready" {
				won.Add(1)
			}
		})
	}
	wg.Wait()
	if won.Load() != 1 {
		t.Fatalf("proof winners: %d", won.Load())
	}
	r, _ = s.Request(t.Context(), r.ID)
	in := claimInput(t, r)
	results := make(chan ClaimResult, 2)
	errs := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Go(func() { v, e := s.Claim(t.Context(), r, in); results <- v; errs <- e })
	}
	wg.Wait()
	close(results)
	close(errs)
	for e := range errs {
		if e != nil {
			t.Fatal(e)
		}
	}
	var token1 string
	for v := range results {
		if token1 != "" && v.Token != token1 {
			t.Fatal("claim retry created another session")
		}
		token1 = v.Token
	}
	var count int
	s.db.QueryRow(t.Context(), `SELECT count(*) FROM account_sessions`).Scan(&count)
	if count != 1 {
		t.Fatal("duplicate sessions")
	}
	if _, err = s.Session(t.Context(), token1); err != nil {
		t.Fatal(err)
	}
}
func TestCrossDeviceCannotPromotePendingPassword(t *testing.T) {
	s := testService(t)
	attacker := requestFor(t, s, "web")
	c, err := s.StartEmail(t.Context(), attacker, "signup", "victim@example.com", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	m := messageFor(t, s, c)
	token := strings.TrimPrefix(m.Link, s.Origin()+"/a/")
	current := requestFor(t, s, "web")
	if _, err = s.VerifyEmail(t.Context(), current, VerifyInput{Link: token}); err == nil {
		t.Fatal("cross-device accepted attacker password")
	}
	v, err := s.VerifyEmail(t.Context(), current, VerifyInput{Link: token, NewPassword: "the real owner has a different password"})
	if err != nil || v.State != "ready" {
		t.Fatal(err)
	}
	old, _ := s.Request(t.Context(), attacker.ID)
	if old.Status != "CANCELLED" {
		t.Fatal("old requester remains claimable")
	}
	login := requestFor(t, s, "web")
	if err = s.PasswordLogin(t.Context(), login, "victim@example.com", testPassword); err == nil {
		t.Fatal("attacker password activated")
	}
	if err = s.PasswordLogin(t.Context(), login, "victim@example.com", "the real owner has a different password"); err != nil {
		t.Fatal(err)
	}
}
func TestResetRevokesSessionsAndLateClaims(t *testing.T) {
	s := testService(t)
	r, _ := signup(t, s, "reset@example.com")
	in := claimInput(t, r)
	claim, err := s.Claim(t.Context(), r, in)
	if err != nil {
		t.Fatal(err)
	}
	late := requestFor(t, s, "web")
	if err = s.PasswordLogin(t.Context(), late, "reset@example.com", testPassword); err != nil {
		t.Fatal(err)
	}
	late, _ = s.Request(t.Context(), late.ID)
	reset := requestFor(t, s, "web")
	c, err := s.StartEmail(t.Context(), reset, "reset", "reset@example.com", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.Delivery != "conditional" {
		t.Fatal("reset enumerates account")
	}
	m := messageFor(t, s, c)
	verified, err := s.VerifyEmail(t.Context(), reset, VerifyInput{ChallengeID: c.ID, Generation: 1, Code: m.Code})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.ResetPassword(t.Context(), reset, verified.ResetGrant, "the new password has 39 small pebbles"); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Session(t.Context(), claim.Token); err == nil {
		t.Fatal("old session survived reset")
	}
	if _, err = s.Claim(t.Context(), late, claimInput(t, late)); err == nil {
		t.Fatal("pre-reset authorization claimed after reset")
	}
	if err = s.ResetPassword(t.Context(), reset, verified.ResetGrant, testPassword); err == nil {
		t.Fatal("reset grant replay")
	}
	unknown := requestFor(t, s, "web")
	u, err := s.StartEmail(t.Context(), unknown, "reset", "missing@example.com", "")
	if err != nil || u.Delivery != c.Delivery {
		t.Fatal("unknown reset has distinguishable response")
	}
}
func TestEmailResendAttemptsAndMailLease(t *testing.T) {
	s := testService(t)
	r := requestFor(t, s, "web")
	c, err := s.StartEmail(t.Context(), r, "signup", "resend@example.com", testPassword)
	if err != nil {
		t.Fatal(err)
	}
	old := messageFor(t, s, c)
	if _, err = s.Resend(t.Context(), r, c.ID, 1); err == nil {
		t.Fatal("cooldown bypass")
	}
	_, _ = s.db.Exec(t.Context(), `UPDATE account_email_challenges SET resend_at=now()-interval '1 second' WHERE id=$1`, c.ID)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Go(func() { _, _ = s.Resend(t.Context(), r, c.ID, 1) })
	}
	wg.Wait()
	latest, err := s.EmailStatus(t.Context(), r, c.ID)
	if err != nil || latest.Generation != 2 {
		t.Fatal("resend generation raced")
	}
	newMail := messageFor(t, s, latest)
	if _, err = s.InspectLink(t.Context(), strings.TrimPrefix(old.Link, s.Origin()+"/a/")); err == nil {
		t.Fatal("old link survived resend")
	}
	start, release := make(chan struct{}), make(chan struct{})
	sender := &fakeSender{gate: func() { close(start); <-release }}
	s.sender = sender
	done := make(chan struct{})
	go func() { _, _ = s.mailOne(t.Context()); close(done) }()
	<-start
	if worked, err := s.mailOne(t.Context()); err != nil || worked {
		t.Fatal("two workers sent same generation")
	}
	close(release)
	<-done
	if sender.count.Load() != 1 {
		t.Fatal("mail lease failed")
	}
	for i := 0; i < 5; i++ {
		_, err = s.VerifyEmail(t.Context(), r, VerifyInput{ChallengeID: c.ID, Generation: 2, Code: "invalid!"})
		if err == nil {
			t.Fatal("wrong code accepted")
		}
	}
	if _, err = s.InspectLink(t.Context(), strings.TrimPrefix(newMail.Link, s.Origin()+"/a/")); err == nil {
		t.Fatal("exhausted challenge accepted")
	}
}

type fakeGoogle struct {
	subject   string
	exchanges atomic.Int32
}

func (f *fakeGoogle) URL(state, nonce, verifier string) string {
	return "https://accounts.google.com/?state=" + url.QueryEscape(state)
}
func (f *fakeGoogle) Exchange(ctx context.Context, code, verifier, nonce string) (GoogleClaims, error) {
	f.exchanges.Add(1)
	return GoogleClaims{Subject: f.subject, Email: "google@example.com", Name: "Google person"}, nil
}
func TestGoogleFirstLoginAndNativeApproval(t *testing.T) {
	s := testService(t)
	provider := &fakeGoogle{subject: "provider-subject"}
	s.google = provider
	var first string
	for _, kind := range []string{"web", "native"} {
		r := requestFor(t, s, kind)
		link, err := s.GoogleStart(t.Context(), r)
		if err != nil {
			t.Fatal(err)
		}
		u, _ := url.Parse(link)
		state := u.Query().Get("state")
		if _, err = s.GoogleOpen(t.Context(), state, "test-browser"); err != nil {
			t.Fatal(err)
		}
		if _, err = s.GoogleCallback(t.Context(), state, "code", "", "wrong-browser"); err == nil {
			t.Fatal("callback browser binding bypass")
		}
		current, err := s.GoogleCallback(t.Context(), state, "code", "", "test-browser")
		if err != nil {
			t.Fatal(err)
		}
		if first == "" {
			first = *current.AccountID
		} else if first != *current.AccountID {
			t.Fatal("Google login duplicated account")
		}
		if _, err = s.GoogleCallback(t.Context(), state, "code", "", "test-browser"); err == nil {
			t.Fatal("Google callback replay")
		}
		if kind == "native" {
			if _, err = s.Claim(t.Context(), current, claimInput(t, current)); err == nil {
				t.Fatal("native Google bypassed approval")
			}
			review, err := s.NativeReview(t.Context(), r.ID, "test-browser")
			if err != nil {
				t.Fatal(err)
			}
			if err = s.NativeConfirm(t.Context(), r.ID, "test-browser", review["csrf"].(string), true); err != nil {
				t.Fatal(err)
			}
		}
	}
	if provider.exchanges.Load() != 2 {
		t.Fatal("callback replay reached provider")
	}
	var passwords int
	s.db.QueryRow(t.Context(), `SELECT count(*) FROM account_passwords`).Scan(&passwords)
	if passwords != 0 {
		t.Fatal("Google unexpectedly required password registration")
	}
}
func TestPasswordBoundsAndTrustedSource(t *testing.T) {
	s := &Service{config: Config{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("10.42.0.0/16")}}, kdfSlots: make(chan struct{}, 2)}
	if s.ClientIP(netip.MustParseAddr("203.0.113.1"), "192.0.2.2") != "203.0.113.1" {
		t.Fatal("untrusted header accepted")
	}
	if s.ClientIP(netip.MustParseAddr("10.42.0.2"), "192.0.2.2") != "192.0.2.2" {
		t.Fatal("trusted edge ignored")
	}
	for _, p := range []string{"short", "passwordpassword", strings.Repeat("x", 128), strings.Repeat("a", 129)} {
		if _, err := passwordValue(p); err == nil {
			t.Fatal("weak or invalid password accepted")
		}
	}
	phc, err := s.passwordHash(t.Context(), testPassword)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := s.passwordVerify(t.Context(), testPassword, phc); err != nil || !ok {
		t.Fatal("password hash verification failed")
	}
	if _, err := s.passwordVerify(t.Context(), testPassword, strings.Replace(phc, "m=32768", "m=999999999", 1)); err == nil {
		t.Fatal("unbounded KDF parameters accepted")
	}
}
