package service

import (
	"errors"
	"testing"
)

func TestSocketRelayRequiresRoleConsentAndActiveDevice(t *testing.T) {
	s, _, _ := testStorage(t)
	tr := newTestTransfer(t, s, []byte("ciphertext"), false)
	for _, c := range []struct{ owner, device, role string }{
		{tr.RecipientOwnerID, tr.RecipientDeviceID, "receiver"},
		{tr.RecipientOwnerID, tr.RecipientDeviceID, "sender"},
		{tr.SenderOwnerID, tr.SenderDeviceID, "receiver"},
		{tr.SenderOwnerID, tr.SenderDeviceID, "other"},
	} {
		if _, err := s.PrepareRelaySocket(t.Context(), tr.ID, c.owner, c.device, c.role); !errors.Is(err, ErrNotFound) {
			t.Fatalf("role/consent accepted: %v", err)
		}
	}
	var count int
	if err := s.db.QueryRow(t.Context(), `SELECT count(*) FROM relay_spools WHERE transfer_id=$1`, tr.ID).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("unauthorized socket created a spool")
	}
	if _, err := s.PrepareRelaySocket(t.Context(), tr.ID, tr.SenderOwnerID, tr.SenderDeviceID, "sender"); err != nil {
		t.Fatal(err)
	}
	tr = transferAction(t, s, tr, "accept")
	if _, err := s.PrepareRelaySocket(t.Context(), tr.ID, tr.RecipientOwnerID, tr.RecipientDeviceID, "receiver"); err != nil {
		t.Fatal(err)
	}
	mustSQL(t, s, `UPDATE devices SET status='REVOKED' WHERE owner_id=$1 AND device_id=$2`, tr.RecipientOwnerID, tr.RecipientDeviceID)
	if _, err := s.PrepareRelaySocket(t.Context(), tr.ID, tr.RecipientOwnerID, tr.RecipientDeviceID, "receiver"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked socket accepted: %v", err)
	}
}
