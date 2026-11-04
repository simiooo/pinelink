package service

import (
	"errors"
	"testing"

	"github.com/simiooo/pinelink/backend/internal/auth"
)

func TestTextTransferRequiresTypedConsent(t *testing.T) {
	s := guestService(t)
	a, b := guestIdentity(t, s), guestIdentity(t, s)
	peer := pairVisitors(t, s, a, b)
	ca, cb := auth.WithIdentity(t.Context(), a), auth.WithIdentity(t.Context(), b)
	req := pairedRequest(a, b, peer.ID)
	req.FileName, req.ContentKind = "Text", "TEXT"
	req.SizeBytes = 33
	created, err := s.CreateP2P(ca, req)
	if err != nil {
		t.Fatal(err)
	}
	if created.ContentKind == nil || *created.ContentKind != "TEXT" {
		t.Fatal("missing content kind")
	}
	pending, err := s.GetTransfer(cb, created.ID, b.OwnerID, b.DeviceID)
	if err != nil || len(pending.EncryptedManifest) != 0 {
		t.Fatal("pending text exposed its key envelope", err)
	}
	action := TransferActionRequest{OwnerID: b.OwnerID, DeviceID: b.DeviceID, ExpectedVersion: created.Version}
	if _, err := s.RespondTransfer(cb, created.ID, "accept", action); !errors.Is(err, ErrInvalid) {
		t.Fatal("legacy client accepted typed text", err)
	}
	stillPending, err := s.GetTransfer(cb, created.ID, b.OwnerID, b.DeviceID)
	if err != nil || stillPending.State != "PENDING" || len(stillPending.EncryptedManifest) != 0 {
		t.Fatal("failed consent changed transfer", err)
	}
	action.AcceptContentKind = "TEXT"
	accepted, err := s.RespondTransfer(cb, created.ID, "accept", action)
	if err != nil || accepted.State != "ACCEPTED" || len(accepted.EncryptedManifest) == 0 {
		t.Fatal("typed consent failed", err)
	}
	action.AcceptContentKind = ""
	if _, err := s.RespondTransfer(cb, created.ID, "accept", action); !errors.Is(err, ErrInvalid) {
		t.Fatal("legacy retry bypassed typed consent", err)
	}
	changed := req
	changed.ContentKind = ""
	if _, err := s.CreateP2P(ca, changed); !errors.Is(err, ErrConflict) {
		t.Fatal("idempotency allowed type change", err)
	}
}
