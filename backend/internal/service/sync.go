package service

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
)

func (s *Service) SyncEvents(ctx context.Context, after int64, limit int, identity ...string) (SyncResponse, error) {
	if len(identity) < 2 || !validIdentity(identity[0]) || !validIdentity(identity[1]) {
		return SyncResponse{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return SyncResponse{}, err
	}
	defer tx.Rollback(ctx)
	var upper int64
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(event_sequence),0) FROM outbox_events`).Scan(&upper); err != nil {
		return SyncResponse{}, err
	}
	if after < 0 {
		if err := tx.Commit(ctx); err != nil {
			return SyncResponse{}, err
		}
		return SyncResponse{Events: []SyncEvent{}, CurrentSequence: upper}, nil
	}
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	query := `SELECT event_sequence, event_id, event_type, aggregate_id, aggregate_version, payload, occurred_at
		FROM outbox_events WHERE event_sequence>$1`
	args := []any{after}
	if len(identity) >= 2 && identity[0] != "" && identity[1] != "" {
		args = append(args, identity[0], identity[1])
		query += ` AND (
			(payload->>'owner_id'=$2 AND (payload->>'device_id'=$3 OR NOT (payload ? 'device_id'))) OR
			(payload->>'sender_owner_id'=$2 AND payload->>'sender_device_id'=$3) OR
			(payload->>'recipient_owner_id'=$2 AND payload->>'recipient_device_id'=$3)
		)`
	}
	args = append(args, upper)
	query += fmt.Sprintf(" AND event_sequence<=$%d", len(args))
	args = append(args, limit)
	query += fmt.Sprintf(" ORDER BY event_sequence LIMIT $%d", len(args))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return SyncResponse{}, err
	}
	defer rows.Close()
	response := SyncResponse{CurrentSequence: after}
	for rows.Next() {
		var event SyncEvent
		if err := rows.Scan(&event.Sequence, &event.EventID, &event.EventType, &event.AggregateID, &event.AggregateVersion, &event.Payload, &event.OccurredAt); err != nil {
			return SyncResponse{}, err
		}
		// Older persisted events may contain envelopes. Never replay those secrets.
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(event.Payload, &payload); err != nil {
			return SyncResponse{}, err
		}
		delete(payload, "encrypted_manifest")
		delete(payload, "sender_encrypted_manifest")
		event.Payload, err = json.Marshal(payload)
		if err != nil {
			return SyncResponse{}, err
		}
		response.Events = append(response.Events, event)
		response.CurrentSequence = event.Sequence
	}
	if err := rows.Err(); err != nil {
		return SyncResponse{}, err
	}
	if len(response.Events) == 0 && upper > after {
		response.CurrentSequence = upper
	}
	if err := tx.Commit(ctx); err != nil {
		return SyncResponse{}, err
	}
	return response, nil
}
