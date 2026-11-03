package accounts

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"filippo.io/age"
	"sync"
	"testing"
	"time"
)

func guestProof(t *testing.T, s *Service, key ed25519.PrivateKey, device, enc string) (GuestEnrollInput, []byte, string) {
	t.Helper()
	c, e := s.GuestChallenge(t.Context(), GuestChallengeInput{SigningKey: base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey)), Kind: "web", Surface: "web"})
	if e != nil {
		t.Fatal(e)
	}
	in := GuestEnrollInput{Challenge: c["challenge"].(string), DeviceID: device, EncryptionKey: enc, Name: "MacBook Pro"}
	b, _ := json.Marshal(in)
	h := sha256.Sum256(b)
	sig := ed25519.Sign(key, []byte("pinelink-guest-enroll-v1\nPOST\n/v1/guest/enroll\n"+hex.EncodeToString(h[:])))
	return in, b, base64.StdEncoding.EncodeToString(sig)
}
func TestGuestConcurrentEnrollmentReplayAndLeaseFencing(t *testing.T) {
	s := testService(t)
	s.config.GuestsEnabled = true
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	ageKey, _ := age.GenerateX25519Identity()
	enc := ageKey.Recipient().String()
	in, b, sig := guestProof(t, s, key, "guest-browser", enc)
	results := make(chan GuestPrincipal, 8)
	errors := make(chan error, 8)
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			p, e := s.EnrollGuest(context.Background(), in, b, sig, GuestClientContext{Surface: "web"})
			results <- p
			errors <- e
		})
	}
	wg.Wait()
	close(results)
	close(errors)
	for e := range errors {
		if e != nil {
			t.Fatal(e)
		}
	}
	var first GuestPrincipal
	for p := range results {
		if first.OwnerID == "" {
			first = p
		}
		if p.OwnerID != first.OwnerID || p.SessionID != first.SessionID || p.Token != first.Token {
			t.Fatal("replay rotated identity or lease")
		}
	}
	var accounts int
	if e := s.db.QueryRow(t.Context(), `SELECT count(*) FROM accounts`).Scan(&accounts); e != nil || accounts != 0 {
		t.Fatal("guest silently became an account", e)
	}
	if _, e := s.GuestSession(t.Context(), first.Token); e != nil {
		t.Fatal(e)
	}
	in, b, sig = guestProof(t, s, key, "guest-browser", enc)
	renewed, e := s.EnrollGuest(t.Context(), in, b, sig, GuestClientContext{Surface: "web"})
	if e != nil || renewed.SessionID != first.SessionID {
		t.Fatal("live lease did not renew idempotently", e)
	}
	if _, e = s.db.Exec(t.Context(), `UPDATE guest_sessions SET expires_at=now()-interval '1 second' WHERE id=$1`, first.SessionID); e != nil {
		t.Fatal(e)
	}
	in, b, sig = guestProof(t, s, key, "guest-browser", enc)
	fresh, e := s.EnrollGuest(t.Context(), in, b, sig, GuestClientContext{Surface: "web"})
	if e != nil || fresh.OwnerID != first.OwnerID || fresh.SessionID == first.SessionID {
		t.Fatal("expired lease was revived or changed owner", e)
	}
	if _, e = s.GuestSession(t.Context(), first.Token); e == nil {
		t.Fatal("expired token revived")
	}
	if _, e = s.db.Exec(t.Context(), `UPDATE devices SET status='REVOKED' WHERE owner_id=$1`, fresh.OwnerID); e != nil {
		t.Fatal(e)
	}
	in, b, sig = guestProof(t, s, key, "guest-browser", enc)
	if _, e = s.EnrollGuest(t.Context(), in, b, sig, GuestClientContext{Surface: "web"}); e == nil {
		t.Fatal("revoked device was reenrolled")
	}
}
func TestGuestInvalidKeysProofAndExpiredChallenge(t *testing.T) {
	s := testService(t)
	s.config.GuestsEnabled = true
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	ageKey, _ := age.GenerateX25519Identity()
	in, b, sig := guestProof(t, s, key, "guest-browser", ageKey.Recipient().String())
	if _, e := s.EnrollGuest(t.Context(), in, b, sig, GuestClientContext{Surface: "app"}); e == nil {
		t.Fatal("wrong surface enrolled identity")
	}
	var count int
	if e := s.db.QueryRow(t.Context(), `SELECT count(*) FROM guest_identities`).Scan(&count); e != nil || count != 0 {
		t.Fatal("rejected surface changed identity state", e)
	}
	if _, e := s.EnrollGuest(t.Context(), in, append(b, ' '), sig, GuestClientContext{Surface: "web"}); e == nil {
		t.Fatal("proof accepted a different body")
	}
	s.now = func() time.Time { return time.Now().Add(2 * time.Minute) }
	if _, e := s.EnrollGuest(t.Context(), in, b, sig, GuestClientContext{Surface: "web"}); e == nil {
		t.Fatal("expired challenge accepted")
	}
	s.now = time.Now
	signing := base64.StdEncoding.EncodeToString(key.Public().(ed25519.PublicKey))
	if _, e := s.GuestChallenge(t.Context(), GuestChallengeInput{SigningKey: signing + "\n", Kind: "web", Surface: "web"}); e == nil {
		t.Fatal("noncanonical signing key accepted")
	}
}
