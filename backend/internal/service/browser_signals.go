package service

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/pion/sdp/v3"
	"github.com/simiooo/pinelink/backend/internal/auth"
)

// Browser signalling is a bounded metadata plane. It never grants permission
// to receive file bytes or opens the native/relay signalling families to guests.
func browserSignal(channel string) bool {
	return channel == "web-direct-offer" || channel == "web-direct-answer" || channel == "web-direct-failed" || channel == "web-direct-failed-sender"
}

func browserSignalRole(channel, role string) bool {
	return role == "SENDER" && (channel == "web-direct-offer" || channel == "web-direct-failed-sender") || role == "RECEIVER" && (channel == "web-direct-answer" || channel == "web-direct-failed")
}

func (s *Service) guestSignalAccess(ctx context.Context, tx pgx.Tx, tr Transfer) error {
	i, ok := auth.FromContext(ctx)
	if !ok || i.GuestSessionID == "" {
		return nil
	}
	if !participant(tr, i.OwnerID, i.DeviceID) {
		return ErrNotFound
	}
	var pair string
	err := tx.QueryRow(ctx, `SELECT p.id::text FROM guest_pair_grants p JOIN transfer_requests tr ON tr.guest_pair_id=p.id
		WHERE tr.id=$1 AND p.revoked_at IS NULL AND p.expires_at>clock_timestamp()
		AND NOT EXISTS(SELECT 1 FROM guest_identities g LEFT JOIN guest_sessions gs ON gs.id=g.current_session_id
			WHERE g.owner_id IN (tr.sender_owner_id,tr.recipient_owner_id) AND (g.revoked_at IS NOT NULL OR gs.id IS NULL OR gs.revoked_at IS NOT NULL OR gs.expires_at<=clock_timestamp())) FOR SHARE OF p`, tr.ID).Scan(&pair)
	if err == pgx.ErrNoRows {
		return ErrUnauthorized
	}
	return err
}

func (s *Service) validateBrowserSignal(ctx context.Context, tx pgx.Tx, tr Transfer, req P2PSignalRequest, role string) error {
	if tr.DeliveryMode != "ONLINE" || tr.ContentRoot == nil {
		return ErrInvalid
	}
	if !browserSignalRole(req.Channel, role) || len(req.Payload) == 0 || len(req.Payload) > 32<<10 {
		return ErrInvalid
	}
	if err := s.guestSignalAccess(ctx, tx, tr); err != nil {
		return err
	}
	if strings.HasPrefix(req.Channel, "web-direct-failed") {
		if req.Payload != "unavailable" || req.ReplyTo != 0 {
			return ErrInvalid
		}
	} else {
		var description sdp.SessionDescription
		if err := description.Unmarshal([]byte(req.Payload)); err != nil || len(description.MediaDescriptions) != 1 {
			return ErrInvalid
		}
		media := description.MediaDescriptions[0]
		if media.MediaName.Media != "application" || !strings.Contains(strings.Join(media.MediaName.Protos, "/"), "SCTP") {
			return ErrInvalid
		}
		attributes := append(append([]sdp.Attribute{}, description.Attributes...), media.Attributes...)
		keys := map[string]bool{}
		candidates := 0
		for _, a := range attributes {
			keys[a.Key] = true
			if a.Key == "candidate" {
				candidates++
			}
		}
		if candidates > 32 || !keys["fingerprint"] || !keys["ice-ufrag"] || !keys["ice-pwd"] || !keys["sctp-port"] {
			return ErrInvalid
		}
		var failed bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM peer_signals WHERE transfer_id=$1 AND channel IN ('web-direct-failed','web-direct-failed-sender'))`, tr.ID).Scan(&failed); err != nil {
			return err
		}
		if failed {
			return ErrConflict
		}
		if req.Channel == "web-direct-answer" {
			var latest int64
			if err := tx.QueryRow(ctx, `SELECT COALESCE(max(sequence),0) FROM peer_signals WHERE transfer_id=$1 AND channel='web-direct-offer' AND sender_role='SENDER'`, tr.ID).Scan(&latest); err != nil {
				return err
			}
			if latest == 0 || req.ReplyTo != latest {
				return ErrInvalid
			}
		} else if req.ReplyTo != 0 {
			return ErrInvalid
		}
	}
	// AddSignal holds the transfer row, so concurrent submissions cannot exceed
	// this retained-message limit. HTTP rates additionally bound polling/attempts.
	var count, bytes int
	if err := tx.QueryRow(ctx, `SELECT count(*),COALESCE(sum(octet_length(payload)),0) FROM peer_signals WHERE transfer_id=$1 AND channel IN ('web-direct-offer','web-direct-answer','web-direct-failed','web-direct-failed-sender')`, tr.ID).Scan(&count, &bytes); err != nil {
		return err
	}
	if count >= 32 || bytes+len(req.Payload) > 128<<10 {
		return ErrQuotaExceeded
	}
	return nil
}

func releaseBrowserSignals(ctx context.Context, tx pgx.Tx, id string) error {
	// The caller holds the transfer row. Use the transfer index and the same
	// terminal transaction, rather than scanning all native signalling each tick.
	_, err := tx.Exec(ctx, `DELETE FROM peer_signals WHERE transfer_id=$1 AND channel IN ('web-direct-offer','web-direct-answer','web-direct-failed','web-direct-failed-sender')`, id)
	return err
}

func (s *Service) guestSignals(ctx context.Context, id, owner, device string, after int64) (P2PSignalList, error) {
	i, _ := auth.FromContext(ctx)
	if owner != i.OwnerID || device != i.DeviceID {
		return P2PSignalList{}, ErrUnauthorized
	}
	tx, tr, err := s.beginTransferObjects(ctx, id)
	if err != nil {
		return P2PSignalList{}, err
	}
	defer tx.Rollback(ctx)
	if !participant(tr, owner, device) {
		return P2PSignalList{}, ErrNotFound
	}
	if tr.State != "PENDING" && tr.State != "ACCEPTED" || !s.now().Before(tr.ExpiresAt) {
		return P2PSignalList{}, ErrConflict
	}
	if err = s.guestSignalAccess(ctx, tx, tr); err != nil {
		return P2PSignalList{}, err
	}
	if err = transferUploadAllowed(ctx, tx, tr); err != nil {
		return P2PSignalList{}, err
	}
	rows, err := tx.Query(ctx, `SELECT sequence,sender_role,channel,payload,created_at,reply_to FROM peer_signals
		WHERE transfer_id=$1 AND sequence>$2 AND channel IN ('web-direct-offer','web-direct-answer','web-direct-failed','web-direct-failed-sender') ORDER BY sequence LIMIT 32`, id, after)
	if err != nil {
		return P2PSignalList{}, err
	}
	result := P2PSignalList{}
	for rows.Next() {
		var signal P2PSignal
		if err = rows.Scan(&signal.Sequence, &signal.SenderRole, &signal.Channel, &signal.Payload, &signal.CreatedAt, &signal.ReplyTo); err != nil {
			rows.Close()
			return P2PSignalList{}, err
		}
		result.Signals = append(result.Signals, signal)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return P2PSignalList{}, err
	}
	return result, tx.Commit(ctx)
}
