package service

import (
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/simiooo/pinelink/backend/internal/auth"
)

func browserSDP() string {
	return strings.Join([]string{"v=0", "o=- 1 1 IN IP4 127.0.0.1", "s=-", "t=0 0", "a=group:BUNDLE 0", "m=application 9 UDP/DTLS/SCTP webrtc-datachannel", "c=IN IP4 0.0.0.0", "a=mid:0", "a=ice-ufrag:abcd", "a=ice-pwd:abcdefghijklmnopqrstuvwxyz", "a=fingerprint:sha-256 " + strings.Repeat("00:", 31) + "00", "a=setup:actpass", "a=sctp-port:5000", "a=candidate:1 1 UDP 2122260223 192.0.2.1 1234 typ host", ""}, "\r\n")
}
func browserRequest(i auth.Identity, channel string, reply int64) P2PSignalRequest {
	role := "SENDER"
	if channel == "web-direct-answer" || channel == "web-direct-failed" {
		role = "RECEIVER"
	}
	payload := browserSDP()
	if strings.HasPrefix(channel, "web-direct-failed") {
		payload = "unavailable"
	}
	return P2PSignalRequest{OwnerID: i.OwnerID, DeviceID: i.DeviceID, Role: role, Channel: channel, Payload: payload, ReplyTo: reply}
}
func TestGuestBrowserSignallingBindsPairRoleOfferAndFileConsent(t *testing.T) {
	s := guestService(t)
	a, b, c := guestIdentity(t, s), guestIdentity(t, s), guestIdentity(t, s)
	ca, cb, cc := auth.WithIdentity(t.Context(), a), auth.WithIdentity(t.Context(), b), auth.WithIdentity(t.Context(), c)
	pair := pairVisitors(t, s, a, b)
	tr, err := s.CreateP2P(ca, pairedRequest(a, b, pair.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddSignal(ca, tr.ID, browserRequest(b, "web-direct-answer", 0)); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("forged request principal", err)
	}
	if _, err = s.Signals(cc, tr.ID, c.OwnerID, c.DeviceID, 0); !errors.Is(err, ErrNotFound) {
		t.Fatal("foreign participant read metadata", err)
	}
	if _, err = s.Signals(cc, tr.ID, a.OwnerID, a.DeviceID, 0); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("forged query principal", err)
	}
	classic := browserRequest(a, "direct-offer", 0)
	if _, err = s.AddSignal(ca, tr.ID, classic); !errors.Is(err, ErrInvalid) {
		t.Fatal("guest entered native signalling", err)
	}
	for _, payload := range []string{"not SDP", strings.Repeat("x", (32<<10)+1), strings.Replace(browserSDP(), "m=application", "m=audio", 1)} {
		bad := browserRequest(a, "web-direct-offer", 0)
		bad.Payload = payload
		if _, err = s.AddSignal(ca, tr.ID, bad); !errors.Is(err, ErrInvalid) {
			t.Fatal("unbounded or non-data SDP accepted", err)
		}
	}
	first, err := s.AddSignal(ca, tr.ID, browserRequest(a, "web-direct-offer", 0))
	if err != nil {
		t.Fatal(err)
	}
	latest, err := s.AddSignal(ca, tr.ID, browserRequest(a, "web-direct-offer", 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddSignal(cb, tr.ID, browserRequest(b, "web-direct-answer", first.Sequence)); !errors.Is(err, ErrInvalid) {
		t.Fatal("late answer escaped current offer", err)
	}
	if _, err = s.AddSignal(cb, tr.ID, browserRequest(b, "web-direct-answer", latest.Sequence)); err != nil {
		t.Fatal(err)
	}
	list, err := s.Signals(cb, tr.ID, b.OwnerID, b.DeviceID, 0)
	if err != nil || len(list.Signals) != 3 {
		t.Fatal("pending peer metadata", list, err)
	}
	if _, err = s.Signals(cb, tr.ID, b.OwnerID, b.DeviceID, -1); !errors.Is(err, ErrInvalid) {
		t.Fatal("negative signal cursor", err)
	}
	var conflict *StateConflictError
	if err = s.UpdateP2PProgress(cb, tr.ID, P2PProgressRequest{OwnerID: b.OwnerID, DeviceID: b.DeviceID, ReceivedBytes: 16, Route: "DIRECT", RouteEpoch: 1}); !errors.As(err, &conflict) {
		t.Fatal("negotiation bypassed file consent", err)
	}
	if _, err = s.MintRelayTicket(cb, tr.ID, "receiver"); err == nil {
		t.Fatal("pending receiver opened file data plane")
	}
	if _, err = s.AddSignal(ca, tr.ID, browserRequest(a, "web-direct-failed-sender", 0)); err != nil {
		t.Fatal(err)
	}
	if _, err = s.AddSignal(cb, tr.ID, browserRequest(b, "web-direct-answer", latest.Sequence)); !errors.Is(err, ErrConflict) {
		t.Fatal("late answer reversed cloud fallback", err)
	}
	if _, err = s.AddSignal(ca, tr.ID, browserRequest(a, "web-direct-offer", 0)); !errors.Is(err, ErrConflict) {
		t.Fatal("new offer reversed cloud fallback", err)
	}
	if _, err = s.RespondTransfer(cb, tr.ID, "reject", TransferActionRequest{OwnerID: b.OwnerID, DeviceID: b.DeviceID, ExpectedVersion: tr.Version}); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Signals(ca, tr.ID, a.OwnerID, a.DeviceID, 0); !errors.Is(err, ErrConflict) {
		t.Fatal("terminal metadata remained readable", err)
	}
	var remaining int
	if err = s.db.QueryRow(t.Context(), `SELECT count(*) FROM peer_signals WHERE transfer_id=$1`, tr.ID).Scan(&remaining); err != nil || remaining != 0 {
		t.Fatal("terminal signalling deletion was not atomic", remaining, err)
	}
	if _, err = s.db.Exec(t.Context(), `INSERT INTO peer_signals(transfer_id,sender_role,channel,payload) VALUES($1,'SENDER','direct-offer','native history')`, tr.ID); err != nil {
		t.Fatal(err)
	}
	if err := NewMaintenance(s.db, s.blobs, 0, nil).runOnce(t.Context()); err != nil {
		t.Fatal(err)
	}
	var web, native int
	if err = s.db.QueryRow(t.Context(), `SELECT count(*) FILTER(WHERE channel LIKE 'web-direct-%'),count(*) FILTER(WHERE channel='direct-offer') FROM peer_signals WHERE transfer_id=$1`, tr.ID).Scan(&web, &native); err != nil || web != 0 || native != 1 {
		t.Fatal("SDP lifecycle cleanup", web, native, err)
	}
}
func TestGuestBrowserSignalCountIsAtomic(t *testing.T) {
	s := guestService(t)
	a, b := guestIdentity(t, s), guestIdentity(t, s)
	ca := auth.WithIdentity(t.Context(), a)
	pair := pairVisitors(t, s, a, b)
	tr, err := s.CreateP2P(ca, pairedRequest(a, b, pair.ID))
	if err != nil {
		t.Fatal(err)
	}
	var wins atomic.Int32
	var wg sync.WaitGroup
	for range 40 {
		wg.Go(func() {
			if _, err := s.AddSignal(ca, tr.ID, browserRequest(a, "web-direct-offer", 0)); err == nil {
				wins.Add(1)
			} else if !errors.Is(err, ErrQuotaExceeded) {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if wins.Load() != 32 {
		t.Fatal("concurrent retained-signal limit", wins.Load())
	}
	list, err := s.Signals(auth.WithIdentity(t.Context(), b), tr.ID, b.OwnerID, b.DeviceID, 0)
	if err != nil || len(list.Signals) != 32 {
		t.Fatal("bounded read", len(list.Signals), err)
	}
}
func TestGuestBrowserSignalsRespectPairLeaseAndTransferDeadlines(t *testing.T) {
	for _, kind := range []string{"pair", "peer-session", "transfer"} {
		t.Run(kind, func(t *testing.T) {
			s := guestService(t)
			a, b := guestIdentity(t, s), guestIdentity(t, s)
			ca := auth.WithIdentity(t.Context(), a)
			pair := pairVisitors(t, s, a, b)
			tr, err := s.CreateP2P(ca, pairedRequest(a, b, pair.ID))
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.AddSignal(ca, tr.ID, browserRequest(a, "web-direct-offer", 0)); err != nil {
				t.Fatal(err)
			}
			switch kind {
			case "pair":
				_, err = s.db.Exec(t.Context(), `UPDATE guest_pair_grants SET expires_at=now()-interval '1 second' WHERE id=$1`, pair.ID)
			case "peer-session":
				_, err = s.db.Exec(t.Context(), `UPDATE guest_sessions SET revoked_at=now() WHERE id=$1`, b.GuestSessionID)
			case "transfer":
				_, err = s.db.Exec(t.Context(), `UPDATE transfer_requests SET expires_at=now()-interval '1 second' WHERE id=$1`, tr.ID)
			}
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Signals(ca, tr.ID, a.OwnerID, a.DeviceID, 0); err == nil {
				t.Fatal("expired or revoked metadata read")
			}
			if _, err = s.AddSignal(ca, tr.ID, browserRequest(a, "web-direct-offer", 0)); err == nil {
				t.Fatal("expired or revoked metadata write")
			}
		})
	}
}
