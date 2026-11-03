package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type ByteObservation struct {
	State             string     `json:"state"`
	Basis             string     `json:"basis"`
	AcknowledgedBytes int64      `json:"acknowledged_bytes"`
	TotalBytes        *int64     `json:"total_bytes"`
	Revision          int64      `json:"revision"`
	SampledAt         *time.Time `json:"sampled_at"`
}

type SpoolObservation struct {
	Generation string `json:"generation,omitempty"`
	ByteObservation
	StateVersion int64      `json:"state_version"`
	SealedAt     *time.Time `json:"sealed_at"`
}

type RouteObservation struct {
	Kind  string `json:"kind"`
	Epoch int64  `json:"epoch"`
}

type P2PProgressSnapshot struct {
	TransferID      string `json:"transfer_id"`
	RevocationEpoch int64  `json:"revocation_epoch"`
	// Each existing P2P transfer is one immutable content attempt. Retry creates a new transfer.
	AttemptID        string           `json:"attempt_id"`
	State            string           `json:"state"`
	StateVersion     int64            `json:"state_version"`
	ProgressRevision int64            `json:"progress_revision"`
	ControlRevision  int64            `json:"control_revision"`
	Direction        string           `json:"direction"`
	DisplayMetricID  string           `json:"display_metric_id"`
	ExpiresAt        time.Time        `json:"expires_at"`
	Route            RouteObservation `json:"route"`
	Delivery         ByteObservation  `json:"delivery"`
	Spool            SpoolObservation `json:"spool"`
	Completion       struct {
		Delivered bool `json:"delivered"`
		Stored    bool `json:"stored"`
	} `json:"completion"`
	Capabilities struct {
		Pause   bool `json:"pause"`
		Resume  bool `json:"resume"`
		Cancel  bool `json:"cancel"`
		Restore bool `json:"restore"`
	} `json:"capabilities"`
}

func (s *Service) GetP2PProgress(ctx context.Context, id, ownerID, deviceID string) (P2PProgressSnapshot, error) {
	// Every query observes the same DB snapshot; no mix of old delivery and new spool/terminal state.
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return P2PProgressSnapshot{}, err
	}
	defer tx.Rollback(ctx)
	tr, err := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM transfer_requests WHERE id=$1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return P2PProgressSnapshot{}, ErrNotFound
	}
	if err != nil {
		return P2PProgressSnapshot{}, err
	}
	if !participant(tr, ownerID, deviceID) {
		return P2PProgressSnapshot{}, ErrNotFound
	}
	if tr.ContentRoot == nil {
		return P2PProgressSnapshot{}, ErrInvalid
	}
	var observedAt time.Time
	if err = tx.QueryRow(ctx, `SELECT now()`).Scan(&observedAt); err != nil {
		return P2PProgressSnapshot{}, err
	}
	task, err := scanTask(tx.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE kind='DOWNLOAD' AND reference_id=$1`, id))
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return P2PProgressSnapshot{}, err
	}
	snap := P2PProgressSnapshot{TransferID: tr.ID, AttemptID: tr.ID, State: tr.State, StateVersion: tr.Version, RevocationEpoch: tr.RevocationEpoch,
		ControlRevision:  task.Version,
		ProgressRevision: tr.ProgressRevision, Direction: "SEND", DisplayMetricID: "delivery:" + tr.ID + ":" + tr.RecipientDeviceID,
		ExpiresAt: tr.ExpiresAt, Route: RouteObservation{Kind: tr.ActiveRoute, Epoch: tr.RouteEpoch}}
	if ownerID == tr.RecipientOwnerID && deviceID == tr.RecipientDeviceID {
		snap.Direction = "RECEIVE"
	}
	snap.Delivery = ByteObservation{State: tr.State, Basis: "RECIPIENT_CIPHERTEXT_ACKNOWLEDGED", AcknowledgedBytes: tr.ReceivedBytes,
		TotalBytes: tr.SizeBytes, Revision: tr.ProgressRevision, SampledAt: tr.ProgressSampledAt}
	snap.Completion.Delivered = tr.State == "COMPLETED"
	switch tr.State {
	case "PENDING":
		snap.Delivery.State = "WAITING_FOR_ACCEPTANCE"
	case "COMPLETED":
		snap.Delivery.State = "DELIVERED"
	case "ACCEPTED":
		snap.Delivery.State = task.State
		if !observedAt.Before(tr.ExpiresAt) {
			snap.Delivery.State = "EXPIRED"
			snap.State = "EXPIRED"
		} else if task.State == "RUNNING" && tr.SizeBytes != nil && tr.ReceivedBytes == *tr.SizeBytes {
			snap.Delivery.State = "VERIFYING"
		}
	}
	if tr.State == "PENDING" && !observedAt.Before(tr.ExpiresAt) {
		snap.Delivery.State = "EXPIRED"
		snap.State = "EXPIRED"
	}
	// Existing controls belong to the receiver task; do not advertise sender controls until delegated commands exist.
	receiver := snap.Direction == "RECEIVE"
	active := snap.State == "ACCEPTED"
	snap.Capabilities.Pause = receiver && active && task.State == "RUNNING" && snap.Delivery.State != "VERIFYING"
	snap.Capabilities.Resume = receiver && active && task.State == "PAUSED"
	snap.Capabilities.Cancel = (snap.Direction == "SEND" && (snap.State == "PENDING" || active))
	snap.Spool.ByteObservation = ByteObservation{State: "NOT_STARTED", Basis: "DURABLE_CIPHERTEXT_CHUNKS"}
	var versionState, uploadState, uploadTaskState string
	var entryState *string
	var copyExpires *time.Time
	var total int64
	err = tx.QueryRow(ctx, `
        SELECT v.state,v.version,v.size_bytes,u.state,COALESCE(t.state,''),v.storage_generation,
            COALESCE((SELECT sum(c.size_bytes) FROM upload_chunks c WHERE c.upload_id=u.id),0),
            (SELECT count(*) FROM upload_chunks c WHERE c.upload_id=u.id),
            (SELECT max(c.created_at) FROM upload_chunks c WHERE c.upload_id=u.id),
            v.sealed_at,e.state,e.expires_at
        FROM relay_spools spool JOIN file_versions v ON v.id=spool.version_id JOIN upload_sessions u ON u.id=spool.upload_id
        LEFT JOIN tasks t ON t.kind='UPLOAD' AND t.reference_id=u.id
        LEFT JOIN file_entries e ON e.id=spool.entry_id
        WHERE spool.transfer_id=$1
    `, id).Scan(&versionState, &snap.Spool.StateVersion, &total, &uploadState, &uploadTaskState, &snap.Spool.Generation,
		&snap.Spool.AcknowledgedBytes, &snap.Spool.Revision, &snap.Spool.SampledAt, &snap.Spool.SealedAt, &entryState, &copyExpires)
	if err == nil {
		snap.Spool.TotalBytes = &total
		switch {
		case snap.State == "EXPIRED":
			snap.Spool.State = "EXPIRED"
		case uploadState == "CANCELLED":
			snap.Spool.State = "CANCELLED"
		case uploadState == "EXPIRED":
			snap.Spool.State = "EXPIRED"
		case versionState == "COLLECTED" || versionState == "GC_PENDING":
			snap.Spool.State = "UNAVAILABLE"
		case entryState != nil && *entryState == "DELETED":
			snap.Spool.State = "UNAVAILABLE"
		case entryState != nil && (*entryState == "EXPIRED" || (copyExpires != nil && !observedAt.Before(*copyExpires))):
			snap.Spool.State = "EXPIRED"
		case versionState == "CORRUPT" || uploadTaskState == "FAILED" || uploadState == "FAILED":
			snap.Spool.State = "FAILED"
		case uploadTaskState == "CANCELLED" || uploadTaskState == "DELETED" || uploadState == "CANCELLED":
			snap.Spool.State = "CANCELLED"
		case versionState == "SEALED" && uploadState == "SEALED":
			snap.Spool.State = "SEALED"
		case uploadTaskState == "PAUSED":
			snap.Spool.State = "PAUSED"
		case versionState == "UPLOADING" && snap.Spool.AcknowledgedBytes == total:
			snap.Spool.State = "VERIFYING"
		default:
			snap.Spool.State = "SPOOLING"
		}
		snap.Completion.Stored = snap.Spool.State == "SEALED"
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return P2PProgressSnapshot{}, err
	} else if tr.ActiveRoute == "DIRECT" {
		snap.Spool.State = "NOT_REQUIRED"
	}
	if tr.State == "COMPLETED" && tr.EntryID != nil && snap.Completion.Stored {
		snap.Capabilities.Restore = hasFileGrant(ctx, tx, *tr.EntryID, tr.ID, ownerID, deviceID) == nil
	}
	if err = tx.Commit(ctx); err != nil {
		return P2PProgressSnapshot{}, err
	}
	return snap, nil
}
