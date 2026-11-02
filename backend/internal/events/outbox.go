package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

type Worker struct {
	db     *pgxpool.Pool
	nats   *nats.Conn
	logger *log.Helper
}

func NewWorker(db *pgxpool.Pool, connection *nats.Conn, logger log.Logger) *Worker {
	return &Worker{db: db, nats: connection, logger: log.NewHelper(logger)}
}

func (w *Worker) Start(ctx context.Context) error {
	js, err := w.nats.JetStream()
	if err != nil {
		return err
	}
	for {
		if err := w.ensureStream(js); err == nil {
			break
		} else {
			w.logger.Warnf("JetStream not ready: %v", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
		}
		count, err := w.publishBatch(ctx, js)
		if err != nil && !errors.Is(err, context.Canceled) {
			w.logger.Errorf("publish outbox: %v", err)
		}
		delay := 200 * time.Millisecond
		if err != nil {
			delay = time.Second
		} else if count > 0 {
			delay = 0
		}
		timer.Reset(delay)
	}
}

func (w *Worker) Stop(context.Context) error {
	return nil
}

func (w *Worker) ensureStream(js nats.JetStreamContext) error {
	config := &nats.StreamConfig{
		Name:      "PINELINK_EVENTS",
		Subjects:  []string{"events.pinelink.>"},
		Storage:   nats.FileStorage,
		Retention: nats.LimitsPolicy,
		MaxAge:    7 * 24 * time.Hour,
		Replicas:  1,
	}
	_, err := js.StreamInfo(config.Name)
	if err == nil {
		_, err = js.UpdateStream(config)
		return err
	}
	if !errors.Is(err, nats.ErrStreamNotFound) {
		return err
	}
	_, err = js.AddStream(config)
	return err
}

func (w *Worker) publishBatch(ctx context.Context, js nats.JetStreamContext) (int, error) {
	rows, err := w.db.Query(ctx, `
		SELECT event_sequence, event_id, event_type, aggregate_id, aggregate_version, payload, occurred_at
		FROM outbox_events WHERE published_at IS NULL
		ORDER BY event_sequence LIMIT 50
	`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	type pending struct {
		id, eventType, aggregateID string
		sequence, version          int64
		payload                    json.RawMessage
		occurredAt                 time.Time
	}
	var events []pending
	for rows.Next() {
		var event pending
		if err := rows.Scan(&event.sequence, &event.id, &event.eventType, &event.aggregateID, &event.version, &event.payload, &event.occurredAt); err != nil {
			return 0, err
		}
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, event := range events {
		envelope, err := json.Marshal(map[string]any{
			"event_id": event.id, "event_type": event.eventType, "sequence": event.sequence,
			"aggregate_id": event.aggregateID, "aggregate_version": event.version,
			"occurred_at": event.occurredAt, "payload": event.payload,
		})
		if err != nil {
			return 0, err
		}
		subject := "events.pinelink." + strings.ToLower(event.eventType)
		if _, err := w.db.Exec(ctx, `UPDATE outbox_events SET publish_attempts=publish_attempts+1 WHERE event_id=$1`, event.id); err != nil {
			return len(events), err
		}
		if _, err = js.Publish(subject, envelope, nats.MsgId(event.id), nats.AckWait(5*time.Second)); err != nil {
			return len(events), fmt.Errorf("publish %s: %w", event.id, err)
		}
		if _, err = w.db.Exec(ctx, `UPDATE outbox_events SET published_at=now() WHERE event_id=$1 AND published_at IS NULL`, event.id); err != nil {
			return len(events), err
		}
	}
	return len(events), nil
}
