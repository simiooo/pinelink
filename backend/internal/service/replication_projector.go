package service

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	rp "github.com/simiooo/pinelink/protocol/replication"
)

type HistoryReplicator struct {
	service  *Service
	interval time.Duration
	logger   *slog.Logger
}

func NewHistoryReplicator(s *Service, interval time.Duration, logger *slog.Logger) *HistoryReplicator {
	if interval <= 0 {
		interval = time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &HistoryReplicator{s, interval, logger}
}
func (w *HistoryReplicator) Start(ctx context.Context) error {
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		if err := w.service.ProjectReplication(ctx); err != nil && ctx.Err() == nil {
			w.logger.Error("history replication projection failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
func (w *HistoryReplicator) Stop(context.Context) error { return nil }

type businessReplicationEvent struct {
	id, kind, aggregate string
	sequence, version   int64
	payload             []byte
	at                  time.Time
}

// A work marker, not a sequence cursor, finds committed business transactions.
// An older nextval that commits late remains unrouted and cannot be skipped.
func (s *Service) ProjectReplication(ctx context.Context) error {
	if len(s.replicationKey) == 0 {
		return nil
	}
	tx, epoch, err := s.replicationTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT event_id::text,event_type,aggregate_id::text,event_sequence,aggregate_version,payload,occurred_at
FROM outbox_events WHERE NOT history_routed ORDER BY event_sequence LIMIT 100 FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return err
	}
	var events []businessReplicationEvent
	for rows.Next() {
		var e businessReplicationEvent
		if err = rows.Scan(&e.id, &e.kind, &e.aggregate, &e.sequence, &e.version, &e.payload, &e.at); err != nil {
			rows.Close()
			return err
		}
		events = append(events, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, e := range events {
		var payload map[string]json.RawMessage
		if err = json.Unmarshal(e.payload, &payload); err != nil {
			return err
		}
		if rp.ValidID(e.aggregate) {
			for _, prefix := range []string{"", "sender_", "recipient_"} {
				var owner, device string
				_ = json.Unmarshal(payload[prefix+"owner_id"], &owner)
				_ = json.Unmarshal(payload[prefix+"device_id"], &device)
				if !validIdentity(owner) || !validIdentity(device) {
					continue
				}
				if _, err = tx.Exec(ctx, "INSERT INTO replication_business_audience(event_id,owner_id,device_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING", e.id, owner, device); err != nil {
					return err
				}
			}
		}
		if _, err = tx.Exec(ctx, "UPDATE outbox_events SET history_routed=true WHERE event_id=$1", e.id); err != nil {
			return err
		}
	}
	rows, err = tx.Query(ctx, `SELECT a.owner_id,a.device_id,r.root_id,e.event_id::text,e.event_type,e.aggregate_id::text,e.event_sequence,e.aggregate_version,e.payload,e.occurred_at
FROM replication_business_audience a JOIN replication_roots r USING(owner_id,device_id)
JOIN devices d ON d.owner_id=r.owner_id AND d.device_id=r.device_id AND d.status='ACTIVE' AND d.signing_public_key=r.signing_key
JOIN outbox_events e ON e.event_id=a.event_id WHERE a.routed_root IS NULL ORDER BY e.event_sequence LIMIT 100`)
	if err != nil {
		return err
	}
	type audience struct {
		owner, device, root string
		e                   businessReplicationEvent
	}
	var audienceRows []audience
	for rows.Next() {
		var a audience
		if err = rows.Scan(&a.owner, &a.device, &a.root, &a.e.id, &a.e.kind, &a.e.aggregate, &a.e.sequence, &a.e.version, &a.e.payload, &a.e.at); err != nil {
			rows.Close()
			return err
		}
		audienceRows = append(audienceRows, a)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	public := s.replicationPublic()
	serverRoot, _ := rp.Root(public)
	for _, a := range audienceRows {
		payload, err := publicBusinessFact(a.e)
		if err != nil {
			return err
		}
		envelope := rp.Envelope{Version: rp.Version, EventID: uuid.NewSHA1(uuid.NameSpaceOID, []byte(a.e.id+"|"+a.root)).String(), SourceRoot: serverRoot, StreamID: epoch, Sequence: a.e.sequence, AggregateID: a.e.aggregate, AggregateVersion: a.e.version, Authority: "SERVER", AuthorityRoot: serverRoot, Kind: "SERVER_" + a.e.kind, Recipients: []string{a.root}, SigningKey: public, ServerPayload: payload}
		envelope.Signature = rp.Sign(s.replicationKey, envelope.Proof())
		if envelope.Validate() != nil {
			return ErrInvalid
		}
		if _, err = insertReplicationEvent(ctx, tx, envelope); err != nil {
			return err
		}
		if err = routeReplicationEvent(ctx, tx, envelope); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, "UPDATE replication_business_audience SET routed_root=$4 WHERE event_id=$1 AND owner_id=$2 AND device_id=$3", a.e.id, a.owner, a.device, a.root); err != nil {
			return err
		}
	}
	if err = processReplicationBackfill(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func publicBusinessFact(e businessReplicationEvent) ([]byte, error) {
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(e.payload, &payload); err != nil {
		return nil, err
	}
	public := map[string]json.RawMessage{}
	// Explicit allowlist: never replay file names, manifests or private keys.
	for _, key := range []string{"state", "state_version", "status", "transfer_id", "entry_id", "version_id", "pull_id", "task_id", "intent", "route", "route_epoch", "spool_state", "retention_class", "expires_at", "delivered_at", "persisted_at", "available_at", "read_at", "progress_revision", "acknowledged_bytes", "delivery_id", "operation", "reason", "purpose"} {
		if value, ok := payload[key]; ok {
			public[key] = value
		}
	}
	return json.Marshal(struct {
		EventID     string                     `json:"event_id"`
		EventType   string                     `json:"event_type"`
		AggregateID string                     `json:"aggregate_id"`
		Version     int64                      `json:"aggregate_version"`
		OccurredAt  time.Time                  `json:"occurred_at"`
		Payload     map[string]json.RawMessage `json:"payload"`
	}{e.id, e.kind, e.aggregate, e.version, e.at.UTC(), public})
}

func processReplicationBackfill(ctx context.Context, tx pgx.Tx) error {
	var left, right string
	var generation, upper, after int64
	err := tx.QueryRow(ctx, `SELECT b.left_root,b.right_root,b.generation,b.upper_sequence,b.after_sequence FROM replication_backfills b
ORDER BY b.left_root,b.right_root,b.generation LIMIT 1`).Scan(&left, &right, &generation, &upper, &after)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	var active bool
	if err = tx.QueryRow(ctx, "SELECT active AND generation=$3 FROM replication_links WHERE left_root=$1 AND right_root=$2", left, right, generation).Scan(&active); err != nil {
		return err
	}
	if !active {
		_, err = tx.Exec(ctx, "DELETE FROM replication_backfills WHERE left_root=$1 AND right_root=$2 AND generation=$3", left, right, generation)
		return err
	}
	rows, err := tx.Query(ctx, `SELECT stored_sequence,body FROM replication_events WHERE stored_sequence>$1 AND stored_sequence<=$2
AND ((source_root=$3 AND peer_root=$4) OR (source_root=$4 AND peer_root=$3)) ORDER BY stored_sequence LIMIT 100`, after, upper, left, right)
	if err != nil {
		return err
	}
	var envelopes []rp.Envelope
	last := after
	for rows.Next() {
		var body []byte
		if err = rows.Scan(&last, &body); err != nil {
			rows.Close()
			return err
		}
		var e rp.Envelope
		if err = json.Unmarshal(body, &e); err != nil {
			rows.Close()
			return err
		}
		envelopes = append(envelopes, e)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, e := range envelopes {
		if err = routeReplicationEvent(ctx, tx, e); err != nil {
			return err
		}
	}
	if len(envelopes) < 100 {
		_, err = tx.Exec(ctx, "DELETE FROM replication_backfills WHERE left_root=$1 AND right_root=$2 AND generation=$3", left, right, generation)
	} else {
		_, err = tx.Exec(ctx, "UPDATE replication_backfills SET after_sequence=$4 WHERE left_root=$1 AND right_root=$2 AND generation=$3", left, right, generation, last)
	}
	return err
}
