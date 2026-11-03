package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/simiooo/pinelink/backend/internal/accounts"
	"github.com/simiooo/pinelink/backend/internal/auth"
	"sync"
	"testing"
	"time"
)

func guestIdentity(t *testing.T, s *Service) auth.Identity {
	t.Helper()
	public, private, _ := ed25519.GenerateKey(rand.Reader)
	enc, _ := age.GenerateX25519Identity()
	c, e := s.accounts.GuestChallenge(t.Context(), accounts.GuestChallengeInput{SigningKey: base64.StdEncoding.EncodeToString(public), Kind: "web", Surface: "web"})
	if e != nil {
		t.Fatal(e)
	}
	in := accounts.GuestEnrollInput{Challenge: c["challenge"].(string), DeviceID: "web-" + uuid.NewString(), EncryptionKey: enc.Recipient().String(), Name: "Web browser"}
	b, _ := json.Marshal(in)
	h := sha256.Sum256(b)
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(private, []byte("pinelink-guest-enroll-v1\nPOST\n/v1/guest/enroll\n"+hex.EncodeToString(h[:]))))
	p, e := s.accounts.EnrollGuest(t.Context(), in, b, sig, accounts.GuestClientContext{Surface: "web"})
	if e != nil {
		t.Fatal(e)
	}
	return auth.Identity{OwnerID: p.OwnerID, DeviceID: p.DeviceID, GuestSessionID: p.SessionID, ResourceIP: "192.0.2.1"}
}
func guestService(t *testing.T) *Service {
	t.Helper()
	s, _, _ := testStorage(t)
	if e := s.ConfigureAccounts(accounts.Config{Required: true, GuestsEnabled: true, Origin: "https://pinelink.example", EncryptionKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestGuestPairCompetitionAndTransferAuthorization(t *testing.T) {
	s := guestService(t)
	a, b, c := guestIdentity(t, s), guestIdentity(t, s), guestIdentity(t, s)
	ctxA, ctxB, ctxC := auth.WithIdentity(t.Context(), a), auth.WithIdentity(t.Context(), b), auth.WithIdentity(t.Context(), c)
	inv, e := s.CreateGuestInvite(ctxA, uuid.NewString())
	if e != nil {
		t.Fatal(e)
	}
	retry, e := s.CreateGuestInvite(ctxA, inv.ID)
	if e != nil || retry.Token != inv.Token || retry.Code != inv.Code {
		t.Fatal("invite retry changed secret", e)
	}
	ca, e := s.JoinGuestInvite(ctxB, "", inv.Code)
	if e != nil {
		t.Fatal(e)
	}
	cb, e := s.JoinGuestInvite(ctxC, inv.Token, "")
	if e != nil {
		t.Fatal(e)
	}
	waiting, e := s.GuestConnections(ctxA)
	if e != nil || len(waiting.Candidates) != 2 || len(waiting.Peers) != 0 {
		t.Fatal("join authorized without confirmation", e)
	}
	if _, e = s.RespondGuestCandidate(ctxB, cb.ID, true); !errors.Is(e, ErrNotFound) {
		t.Fatal("candidate could approve another device", e)
	}
	wins := 0
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, id := range []string{ca.ID, cb.ID} {
		id := id
		wg.Go(func() {
			if _, e := s.RespondGuestCandidate(context.WithValue(context.Background(), struct{}{}, true), id, true); e == nil {
				t.Error("missing identity accepted")
			}
			if _, e := s.RespondGuestCandidate(auth.WithIdentity(context.Background(), a), id, true); e == nil {
				mu.Lock()
				wins++
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	if wins != 1 {
		t.Fatal("invite had multiple winners", wins)
	}
	list, e := s.GuestConnections(ctxA)
	if e != nil || len(list.Peers) != 1 {
		t.Fatal("missing exact peer", e)
	}
	peer := list.Peers[0]
	target := b
	if peer.Owner == c.OwnerID {
		target = c
	}
	req := P2PCreateRequest{SenderOwnerID: a.OwnerID, SenderDeviceID: a.DeviceID, RecipientOwnerID: target.OwnerID, RecipientDeviceID: target.DeviceID, FileName: "test.bin", SizeBytes: 16, ContentRoot: hex.EncodeToString(make([]byte, 32)), EncryptionSuite: "DARE20-AGE-X25519-v1", EncryptedManifest: []byte("manifest"), SenderEncryptedManifest: []byte("sender"), ClientRequestID: uuid.NewString()}
	if _, e = s.CreateP2P(ctxA, req); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("known ids bypassed pairing", e)
	}
	req.GuestPairID = peer.ID
	tr, e := s.CreateP2P(ctxA, req)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.CreateP2P(ctxA, req)
	if e != nil || again.ID != tr.ID {
		t.Fatal("request retry duplicated transfer", e)
	}
	third := guestIdentity(t, s)
	if _, e = s.GetTransfer(auth.WithIdentity(t.Context(), third), tr.ID, third.OwnerID, third.DeviceID); !errors.Is(e, ErrNotFound) {
		t.Fatal("foreign visitor saw transfer", e)
	}
	if _, e = s.JoinGuestInvite(auth.WithIdentity(t.Context(), third), inv.Token, ""); e == nil {
		t.Fatal("consumed code admitted another peer")
	}
	ticket, e := s.MintRelayTicket(ctxA, tr.ID, "sender")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.ConsumeRelayTicket(t.Context(), ticket["ticket"].(string), tr.ID, "receiver"); e == nil {
		t.Fatal("ticket crossed role")
	}
	identity, e := s.ConsumeRelayTicket(t.Context(), ticket["ticket"].(string), tr.ID, "sender")
	if e != nil || identity.GuestSessionID != a.GuestSessionID {
		t.Fatal("ticket lost typed principal", e)
	}
	if _, e = s.ConsumeRelayTicket(t.Context(), ticket["ticket"].(string), tr.ID, "sender"); e == nil {
		t.Fatal("ticket reused")
	}
	if _, e = s.MintRelayTicket(auth.WithIdentity(t.Context(), target), tr.ID, "receiver"); e == nil {
		t.Fatal("pairing bypassed file consent")
	}
	req.ClientRequestID = uuid.NewString()
	req.SizeBytes = s.accounts.GuestMaxFileBytes() * 2
	if _, e = s.CreateP2P(ctxA, req); !errors.Is(e, ErrQuotaExceeded) {
		t.Fatal("oversized anonymous transfer accepted", e)
	}
	tickets := 0
	for range 16 {
		wg.Go(func() {
			_, err := s.MintRelayTicket(ctxA, tr.ID, "sender")
			if err == nil {
				mu.Lock()
				tickets++
				mu.Unlock()
			} else if !errors.Is(err, ErrQuotaExceeded) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if tickets != 8 {
		t.Fatal("concurrent ticket reservations bypassed capacity", tickets)
	}
}
func TestGuestLeaseExpiryFencesDurableCommits(t *testing.T) {
	s := guestService(t)
	i := guestIdentity(t, s)
	ctx := auth.WithIdentity(t.Context(), i)
	tx, e := s.beginAccountTx(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	done := make(chan error, 1)
	go func() {
		_, e := s.db.Exec(context.Background(), `UPDATE guest_sessions SET revoked_at=clock_timestamp() WHERE id=$1`, i.GuestSessionID)
		done <- e
	}()
	select {
	case <-done:
		t.Fatal("revocation crossed live commit")
	case <-time.After(30 * time.Millisecond):
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if late, e := s.beginAccountTx(ctx); !errors.Is(e, ErrUnauthorized) {
		if late != nil {
			late.Rollback(ctx)
		}
		t.Fatal("revoked lease started a late commit", e)
	}
}

func TestGuestEmptyFileBudgetsAreAtomicAndCannotBeRefundedByCancel(t *testing.T) {
	s, _, _ := testStorage(t)
	if e := s.ConfigureAccounts(accounts.Config{Required: true, GuestsEnabled: true, GuestDailyBytes: 128 << 10, Origin: "https://pinelink.example", EncryptionKey: base64.StdEncoding.EncodeToString(make([]byte, 32))}); e != nil {
		t.Fatal(e)
	}
	a, b := guestIdentity(t, s), guestIdentity(t, s)
	ctxA, ctxB := auth.WithIdentity(t.Context(), a), auth.WithIdentity(t.Context(), b)
	invite, e := s.CreateGuestInvite(ctxA, uuid.NewString())
	if e != nil {
		t.Fatal(e)
	}
	candidate, e := s.JoinGuestInvite(ctxB, invite.Token, "")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.RespondGuestCandidate(ctxA, candidate.ID, true); e != nil {
		t.Fatal(e)
	}
	connections, e := s.GuestConnections(ctxA)
	if e != nil {
		t.Fatal(e)
	}
	request := P2PCreateRequest{GuestPairID: connections.Peers[0].ID, SenderOwnerID: a.OwnerID, SenderDeviceID: a.DeviceID, RecipientOwnerID: b.OwnerID, RecipientDeviceID: b.DeviceID, FileName: "empty.txt", SizeBytes: 0, ContentRoot: hex.EncodeToString(make([]byte, 32)), EncryptionSuite: "DARE20-AGE-X25519-v1", EncryptedManifest: []byte("manifest"), SenderEncryptedManifest: []byte("sender")}
	var wg sync.WaitGroup
	var mu sync.Mutex
	var transfers []Transfer
	for range 3 {
		wg.Go(func() {
			req := request
			req.ClientRequestID = uuid.NewString()
			tr, e := s.CreateP2P(auth.WithIdentity(context.Background(), a), req)
			if e == nil {
				mu.Lock()
				transfers = append(transfers, tr)
				mu.Unlock()
			} else if !errors.Is(e, ErrQuotaExceeded) {
				t.Error(e)
			}
		})
	}
	wg.Wait()
	if len(transfers) != 2 {
		t.Fatal("anonymous concurrent reservation crossed budget", len(transfers))
	}
	for _, tr := range transfers {
		if _, e = s.RespondTransfer(ctxA, tr.ID, "cancel", TransferActionRequest{OwnerID: a.OwnerID, DeviceID: a.DeviceID, ExpectedVersion: tr.Version}); e != nil {
			t.Fatal(e)
		}
	}
	request.ClientRequestID = uuid.NewString()
	if _, e = s.CreateP2P(ctxA, request); !errors.Is(e, ErrQuotaExceeded) {
		t.Fatal("cancel refunded a daily anonymous budget", e)
	}
	var reserved int64
	if e = s.db.QueryRow(t.Context(), `SELECT max(bytes_reserved) FROM guest_resource_budgets`).Scan(&reserved); e != nil || reserved != 128<<10 {
		t.Fatal("failed reservation left a partial charge", reserved, e)
	}
}

func TestGuestDeviceRevocationCannotPassAnActiveDataCommit(t *testing.T) {
	s := guestService(t)
	i := guestIdentity(t, s)
	ctx := auth.WithIdentity(t.Context(), i)
	tx, e := s.beginAccountTx(ctx)
	if e != nil {
		t.Fatal(e)
	}
	defer tx.Rollback(ctx)
	done := make(chan error, 1)
	go func() {
		_, e := s.RevokeDevice(auth.WithIdentity(context.Background(), i), DeviceRequest{OwnerID: i.OwnerID, DeviceID: i.DeviceID})
		done <- e
	}()
	select {
	case <-done:
		t.Fatal("device revocation passed an active commit")
	case <-time.After(30 * time.Millisecond):
	}
	if e = tx.Commit(ctx); e != nil {
		t.Fatal(e)
	}
	if e = <-done; e != nil {
		t.Fatal(e)
	}
	if late, e := s.beginAccountTx(ctx); !errors.Is(e, ErrUnauthorized) {
		if late != nil {
			late.Rollback(ctx)
		}
		t.Fatal("revoked device committed with an old visitor lease", e)
	}
}
