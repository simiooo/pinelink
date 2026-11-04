package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/simiooo/pinelink/backend/internal/auth"
)

func accountDevice(t *testing.T, s *Service, owner string) auth.Identity {
	t.Helper()
	if owner == "" {
		owner = "acct-" + uuid.NewString()
		mustSQL(t, s, `INSERT INTO accounts(id) VALUES($1)`, owner)
	}
	device, session := "device-"+uuid.NewString(), uuid.NewString()
	mustSQL(t, s, `INSERT INTO devices(owner_id,device_id,status,public_key,signing_public_key) VALUES($1,$2,'ACTIVE',$3,$4)`, owner, device, "key-"+device, "sign-"+device)
	mustSQL(t, s, `INSERT INTO account_sessions(id,token_hash,account_id,device_id,credential_epoch,expires_at) VALUES($1,$2,$3,$4,1,now()+interval '1 hour')`, session, []byte(uuid.NewString()), owner, device)
	return auth.Identity{OwnerID: owner, DeviceID: device, SessionID: session}
}

func TestAccountTransferRequiresSameAccountOrConfirmedDevicePair(t *testing.T) {
	s := guestService(t)
	a := accountDevice(t, s, "")
	same := accountDevice(t, s, a.OwnerID)
	b := accountDevice(t, s, "")
	other := accountDevice(t, s, "")
	ca, cb := auth.WithIdentity(t.Context(), a), auth.WithIdentity(t.Context(), b)

	request := pairedRequest(a, b, "")
	if _, err := s.CreateP2P(ca, request); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unpaired account created transfer: %v", err)
	}
	if _, err := s.GetRecipientPublicKey(ca, b.OwnerID, b.DeviceID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unpaired account discovered device key: %v", err)
	}
	if _, err := s.GetRecipientPublicKey(ca, same.OwnerID, same.DeviceID); err != nil {
		t.Fatalf("same-account key unavailable: %v", err)
	}
	if _, err := s.CreateP2P(ca, pairedRequest(a, same, "")); err != nil {
		t.Fatalf("same-account active device denied: %v", err)
	}

	pair := pairVisitors(t, s, a, b)
	if _, err := s.CreateP2P(ca, pairedRequest(a, other, pair.ID)); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("pair authorized an unrelated device: %v", err)
	}
	if _, err := s.CreateP2P(ca, pairedRequest(a, same, pair.ID)); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("same-account transfer attached an unrelated pair: %v", err)
	}
	if _, err := s.GetRecipientPublicKey(ca, b.OwnerID, b.DeviceID); err != nil {
		t.Fatalf("paired account key unavailable: %v", err)
	}
	transfer, err := s.CreateP2P(ca, pairedRequest(a, b, pair.ID))
	if err != nil {
		t.Fatalf("paired account transfer denied: %v", err)
	}
	if transfer.State != "PENDING" {
		t.Fatalf("pair bypassed receive consent: %s", transfer.State)
	}
	if _, err = s.RespondTransfer(cb, transfer.ID, "reject", TransferActionRequest{OwnerID: b.OwnerID, DeviceID: b.DeviceID, ExpectedVersion: transfer.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.RemoveGuestConnection(cb, pair.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreateP2P(ca, pairedRequest(a, b, pair.ID)); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked pair created transfer: %v", err)
	}
	if _, err = s.GetRecipientPublicKey(ca, b.OwnerID, b.DeviceID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("revoked pair discovered device key: %v", err)
	}
	if _, err = s.GetTransfer(ca, transfer.ID, a.OwnerID, a.DeviceID); err != nil {
		t.Fatalf("revocation erased transfer history: %v", err)
	}
}

func TestAccountTransferWaitsForTargetOrPairRevocation(t *testing.T) {
	s := guestService(t)
	a := accountDevice(t, s, "")
	same := accountDevice(t, s, a.OwnerID)
	b := accountDevice(t, s, "")
	pair := pairVisitors(t, s, a, b)
	for _, test := range []struct {
		name string
		req  P2PCreateRequest
		stmt string
		args []any
	}{
		{"device", pairedRequest(a, same, ""), `UPDATE devices SET status='REVOKED' WHERE owner_id=$1 AND device_id=$2`, []any{same.OwnerID, same.DeviceID}},
		{"pair", pairedRequest(a, b, pair.ID), `UPDATE guest_pair_grants SET revoked_at=clock_timestamp() WHERE id=$1`, []any{pair.ID}},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx, err := s.db.Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(t.Context())
			if _, err = tx.Exec(t.Context(), test.stmt, test.args...); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() {
				_, err := s.CreateP2P(auth.WithIdentity(context.Background(), a), test.req)
				done <- err
			}()
			select {
			case err = <-done:
				t.Fatalf("creation crossed an uncommitted revocation: %v", err)
			case <-time.After(40 * time.Millisecond):
			}
			if err = tx.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err = <-done; !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("creation survived %s revocation: %v", test.name, err)
			}
		})
	}
}
