package service

import (
	"context"
	"encoding/hex"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/simiooo/pinelink/backend/internal/auth"
)

func pairVisitors(t *testing.T, s *Service, a, b auth.Identity) GuestPeer {
	t.Helper()
	ca, cb := auth.WithIdentity(t.Context(), a), auth.WithIdentity(t.Context(), b)
	invite, err := s.CreateGuestInvite(ca, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := s.JoinGuestInvite(cb, invite.Token, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RespondGuestCandidate(ca, candidate.ID, true); err != nil {
		t.Fatal(err)
	}
	connections, err := s.GuestConnections(ca)
	if err != nil {
		t.Fatal(err)
	}
	for _, peer := range connections.Peers {
		if peer.InviteID == invite.ID {
			return peer
		}
	}
	t.Fatal("confirmed pair missing")
	return GuestPeer{}
}
func pairedRequest(a, b auth.Identity, pair string) P2PCreateRequest {
	return P2PCreateRequest{GuestPairID: pair, SenderOwnerID: a.OwnerID, SenderDeviceID: a.DeviceID, RecipientOwnerID: b.OwnerID, RecipientDeviceID: b.DeviceID, FileName: "test.bin", SizeBytes: 16, ContentRoot: hex.EncodeToString(make([]byte, 32)), EncryptionSuite: "DARE20-AGE-X25519-v1", EncryptedManifest: []byte("manifest"), SenderEncryptedManifest: []byte("sender"), ClientRequestID: uuid.NewString()}
}
func TestGuestConnectionRemovalIsScopedIdempotentAndKeepsHistory(t *testing.T) {
	s := guestService(t)
	a, b, c := guestIdentity(t, s), guestIdentity(t, s), guestIdentity(t, s)
	peer, other := pairVisitors(t, s, a, b), pairVisitors(t, s, a, c)
	ca, cb, cc := auth.WithIdentity(t.Context(), a), auth.WithIdentity(t.Context(), b), auth.WithIdentity(t.Context(), c)
	if _, err := s.RemoveGuestConnection(cc, peer.ID); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign device removed pair", err)
	}
	transfer, err := s.CreateP2P(ca, pairedRequest(a, b, peer.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.RemoveGuestConnection(cb, peer.ID); !errors.Is(err, ErrConflict) {
		t.Fatal("active transfer was detached", err)
	}
	if _, err = s.RespondTransfer(cb, transfer.ID, "reject", TransferActionRequest{OwnerID: b.OwnerID, DeviceID: b.DeviceID, ExpectedVersion: transfer.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RemoveGuestConnection(cb, peer.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RemoveGuestConnection(ca, peer.ID); err != nil {
		t.Fatal("repeat removal changed result", err)
	}
	connections, err := s.GuestConnections(ca)
	if err != nil || len(connections.Peers) != 1 || connections.Peers[0].ID != other.ID {
		t.Fatal("removal affected another pair", err)
	}
	if _, err = s.GetTransfer(ca, transfer.ID, a.OwnerID, a.DeviceID); err != nil {
		t.Fatal("removal erased history", err)
	}
	if _, err = s.CreateP2P(ca, pairedRequest(a, b, peer.ID)); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("removed pair admitted a new transfer", err)
	}
}
func TestGuestTransferCreationWaitsForPairRevocation(t *testing.T) {
	s := guestService(t)
	a, b := guestIdentity(t, s), guestIdentity(t, s)
	peer := pairVisitors(t, s, a, b)
	tx, err := s.db.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(t.Context())
	if _, err = tx.Exec(t.Context(), `UPDATE guest_pair_grants SET revoked_at=clock_timestamp() WHERE id=$1`, peer.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := s.CreateP2P(auth.WithIdentity(context.Background(), a), pairedRequest(a, b, peer.ID))
		done <- err
	}()
	select {
	case err = <-done:
		t.Fatal("creation crossed an uncommitted revocation", err)
	case <-time.After(40 * time.Millisecond):
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err = <-done; !errors.Is(err, ErrUnauthorized) {
		t.Fatal("late creation survived revocation", err)
	}
}
func TestGuestConnectionRemovalAndCreationHaveOneWinner(t *testing.T) {
	s := guestService(t)
	for range 12 {
		a, b := guestIdentity(t, s), guestIdentity(t, s)
		peer := pairVisitors(t, s, a, b)
		start := make(chan struct{})
		var wg sync.WaitGroup
		var creation, removal error
		wg.Go(func() {
			<-start
			_, creation = s.CreateP2P(auth.WithIdentity(context.Background(), a), pairedRequest(a, b, peer.ID))
		})
		wg.Go(func() {
			<-start
			_, removal = s.RemoveGuestConnection(auth.WithIdentity(context.Background(), b), peer.ID)
		})
		close(start)
		wg.Wait()
		if !((creation == nil && errors.Is(removal, ErrConflict)) || (removal == nil && errors.Is(creation, ErrUnauthorized))) {
			t.Fatalf("unserialized result: create=%v remove=%v", creation, removal)
		}
	}
}
