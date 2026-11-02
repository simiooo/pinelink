package lan

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

type session struct {
	peerID   string
	deadline time.Time
	seen     map[string]bool
}
type challenge struct {
	peerID   string
	deadline time.Time
}
type invitation struct {
	token    string
	deadline time.Time
	pairID   string
}
type pendingPair struct {
	request  PairRequest
	deadline time.Time
	token    string
}

type Server struct {
	mu         sync.Mutex
	cfg        Config
	identity   *secure.Identity
	self       Peer
	root       *stateDirectory
	receive    *os.Root
	lock       *os.File
	peers      map[string]Peer
	pairs      map[string]*pendingPair
	invites    map[string]*invitation
	challenges map[string]challenge
	sessions   map[string]*session
	transfers  map[string]*record
	http       *http.Server
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	closed     bool
	closeOnce  sync.Once
	boot       string
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func NewServer(identity *secure.Identity, cfg Config) (*Server, error) {
	if cfg.Directory == "" || cfg.ReceiveDirectory == "" {
		return nil, fmt.Errorf("local state and receive directories are required")
	}
	if cfg.TransferTTL == 0 {
		cfg.TransferTTL = 10 * time.Minute
	}
	if cfg.TransferTTL < time.Second || cfg.TransferTTL > 24*time.Hour {
		return nil, fmt.Errorf("invalid transfer lifetime")
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{cfg: cfg, identity: identity, self: Describe(identity, cfg.Name, cfg.Address), ctx: ctx, cancel: cancel, boot: randomToken(), peers: map[string]Peer{}, pairs: map[string]*pendingPair{}, invites: map[string]*invitation{}, challenges: map[string]challenge{}, sessions: map[string]*session{}, transfers: map[string]*record{}}
	if err := s.self.validate(); err != nil {
		cancel()
		return nil, err
	}
	var err error
	if s.root, err = openStateDirectory(cfg.Directory, identity.LocalID()); err != nil {
		cancel()
		return nil, err
	}
	s.lock, err = s.root.OpenFile("server.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err == nil {
		err = lockDirectory(s.lock)
	}
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("local receiver is already running or cannot lock its state: %w", err)
	}
	if s.receive, err = openDirectory(cfg.ReceiveDirectory); err != nil {
		s.Close()
		return nil, err
	}
	if err = s.loadRecords(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func (s *Server) Prepare(listener net.Listener) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.http != nil {
		return fmt.Errorf("receiver already prepared")
	}
	if s.self.Address == "" {
		address := listener.Addr().(*net.TCPAddr)
		ip := address.IP
		if ip == nil || ip.IsUnspecified() {
			ip = net.ParseIP("127.0.0.1")
		}
		s.self.Address = "https://" + net.JoinHostPort(ip.String(), strconv.Itoa(address.Port))
	}
	if _, err := endpoint(s.self.Address); err != nil {
		return err
	}
	certificate, err := s.identity.LocalCertificate()
	if err != nil {
		return err
	}
	s.http = &http.Server{Handler: http.HandlerFunc(s.handle), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 16 << 10,
		TLSConfig: &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, NextProtos: []string{"http/1.1"}}}
	return writeJSON(s.root, "receiver.json", struct {
		Peer           Peer   `json:"peer"`
		ControlAddress string `json:"control_address"`
	}{s.self, "https://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(listener.Addr().(*net.TCPAddr).Port))})
}

func (s *Server) Peer() Peer { s.mu.Lock(); defer s.mu.Unlock(); return s.self }

func (s *Server) Invite() (Invitation, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.self.Address == "" {
		return Invitation{}, fmt.Errorf("receiver is not listening")
	}
	s.prune()
	if len(s.invites) >= 8 {
		return Invitation{}, fmt.Errorf("too many active invitations")
	}
	token := randomToken()
	s.invites[token] = &invitation{token: token, deadline: time.Now().Add(2 * time.Minute)}
	i := Invitation{Version: Protocol, Peer: s.self, Token: token, ExpiresIn: 120}
	i.Signature = s.identity.Sign(i.proof())
	return i, nil
}

func (s *Server) Serve(ctx context.Context, listener net.Listener) error {
	if s.http == nil {
		if err := s.Prepare(listener); err != nil {
			return err
		}
	}
	go func() {
		select {
		case <-ctx.Done():
			s.Close()
		case <-s.ctx.Done():
		}
	}()
	s.mu.Lock()
	records := make([]*record, 0, len(s.transfers))
	for _, r := range s.transfers {
		records = append(records, r)
	}
	s.mu.Unlock()
	for _, r := range records {
		r.mu.Lock()
		resume := r.t.State == "VERIFYING"
		r.mu.Unlock()
		if resume {
			s.startFinalize(r)
		}
	}
	if s.beginWork() {
		go func() {
			defer s.wg.Done()
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-s.ctx.Done():
					return
				case <-ticker.C:
					s.mu.Lock()
					pending := make([]*record, 0)
					for _, record := range s.transfers {
						if record.active.Load() {
							pending = append(pending, record)
						}
					}
					s.mu.Unlock()
					for _, record := range pending {
						record.mu.Lock()
						_ = s.expireLocked(record)
						record.mu.Unlock()
					}
				}
			}
		}()
	}
	if s.cfg.Sync && s.beginWork() {
		go func() { defer s.wg.Done(); _ = RunSync(s.ctx, s.identity, s.cfg.Directory) }()
	}
	err := s.http.Serve(tls.NewListener(listener, s.http.TLSConfig))
	if errors.Is(err, http.ErrServerClosed) || s.ctx.Err() != nil {
		return nil
	}
	return err
}

func (s *Server) Close() {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.cancel()
		server := s.http
		s.mu.Unlock()
		if server != nil {
			_ = server.Close()
		}
		s.wg.Wait()
		if s.receive != nil {
			_ = s.receive.Close()
		}
		if s.lock != nil {
			_ = s.lock.Close()
		}
		if s.root != nil {
			_ = s.root.Close()
		}
	})
}

func (s *Server) beginWork() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return false
	}
	s.wg.Add(1)
	return true
}
func (s *Server) trusted(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.peerLocked(id)
	return id == s.self.ID || (ok && !p.Revoked)
}
func (s *Server) get(id string) (*record, error) {
	if !validTransferID(id) {
		return nil, fmt.Errorf("invalid transfer id")
	}
	s.mu.Lock()
	r := s.transfers[id]
	s.mu.Unlock()
	if r == nil {
		return nil, os.ErrNotExist
	}
	return r, nil
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if !s.beginWork() {
		http.Error(w, "receiver stopped", 503)
		return
	}
	defer s.wg.Done()
	if r.Header.Get("Origin") != "" {
		localError(w, ErrForbidden)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == "GET" && r.URL.Path == "/local/v1/info" {
		localJSON(w, 200, s.Peer())
		return
	}
	if r.Method == "GET" && r.URL.Path == "/local/v1/challenge" {
		s.handleChallenge(w, r)
		return
	}
	if r.Method == "POST" && r.URL.Path == "/local/v1/session" {
		s.handleSession(w, r)
		return
	}
	if r.Method == "POST" && r.URL.Path == "/local/v1/pair" {
		s.handlePair(w, r)
		return
	}
	if r.Method == "POST" && r.URL.Path == "/local/v1/pair/status" {
		s.handlePairStatus(w, r)
		return
	}
	limit := int64(1 << 20)
	if strings.HasPrefix(r.URL.Path, "/local/v1/replication/") {
		limit = 12 << 20
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit+1))
	if err != nil {
		localError(w, err)
		return
	}
	principal, err := s.authenticate(r, body)
	if err != nil {
		localError(w, err)
		return
	}
	s.route(w, r, principal, body)
}

func localJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func localError(w http.ResponseWriter, err error) {
	status, code := 400, "INVALID_REQUEST"
	switch {
	case errors.Is(err, ErrUnauthorized):
		status, code = 401, "SESSION_REQUIRED"
	case errors.Is(err, ErrForbidden):
		status, code = 403, "FORBIDDEN"
	case errors.Is(err, ErrConflict):
		status, code = 409, "STATE_CONFLICT"
	case errors.Is(err, os.ErrNotExist):
		status, code = 404, "NOT_FOUND"
	}
	localJSON(w, status, map[string]string{"code": code, "message": err.Error()})
}

func decodeRequest(w http.ResponseWriter, r *http.Request, v any) error {
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	return decoder.Decode(v)
}
