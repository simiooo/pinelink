package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

type P2PCreateRequest struct {
	SenderOwnerID           string `json:"sender_owner_id"`
	SenderDeviceID          string `json:"sender_device_id"`
	RecipientOwnerID        string `json:"recipient_owner_id"`
	RecipientDeviceID       string `json:"recipient_device_id"`
	FileName                string `json:"file_name"`
	SizeBytes               int64  `json:"size_bytes"`
	ContentRoot             string `json:"content_root"`
	EncryptionSuite         string `json:"encryption_suite"`
	EncryptedManifest       []byte `json:"encrypted_manifest"`
	SenderEncryptedManifest []byte `json:"sender_encrypted_manifest"`
}

type P2PSignal struct {
	ReplyTo    int64     `json:"reply_to,omitempty"`
	Sequence   int64     `json:"sequence"`
	Channel    string    `json:"channel"`
	Payload    string    `json:"payload"`
	SenderRole string    `json:"sender_role"`
	CreatedAt  time.Time `json:"created_at"`
}

type P2PSignalRequest struct {
	ReplyTo  int64  `json:"reply_to,omitempty"`
	OwnerID  string `json:"owner_id"`
	DeviceID string `json:"device_id"`
	Role     string `json:"role"`
	Channel  string `json:"channel"`
	Payload  string `json:"payload"`
}

type P2PSignalList struct {
	Signals []P2PSignal `json:"signals"`
}

type P2PProgressRequest struct {
	OwnerID       string `json:"owner_id"`
	DeviceID      string `json:"device_id"`
	ReceivedBytes int64  `json:"received_bytes"`
	Route         string `json:"route,omitempty"`
	RouteEpoch    int64  `json:"route_epoch,omitempty"`
}

func (s *Service) CreateP2P(ctx context.Context, req P2PCreateRequest) (Transfer, error) {
	if !validIdentity(req.SenderOwnerID) || !validIdentity(req.SenderDeviceID) ||
		!validIdentity(req.RecipientOwnerID) || !validIdentity(req.RecipientDeviceID) ||
		!validIdentity(req.FileName) || req.SizeBytes < 0 || !validSHA256(req.ContentRoot) || len(req.EncryptedManifest) == 0 || len(req.SenderEncryptedManifest) == 0 {
		return Transfer{}, fmt.Errorf("%w: invalid P2P metadata", ErrInvalid)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return Transfer{}, err
	}
	defer tx.Rollback(ctx)
	transfer := Transfer{ID: uuid.NewString(), DeliveryMode: "ONLINE", SenderOwnerID: req.SenderOwnerID,
		SenderDeviceID: req.SenderDeviceID, RecipientOwnerID: req.RecipientOwnerID, RecipientDeviceID: req.RecipientDeviceID,
		State: "PENDING", Version: 1, FileName: &req.FileName, SizeBytes: &req.SizeBytes, ContentRoot: &req.ContentRoot,
		EncryptionSuite: &req.EncryptionSuite, EncryptedManifest: req.EncryptedManifest, SenderEncryptedManifest: req.SenderEncryptedManifest, ExpiresAt: s.now().Add(10 * time.Minute)}
	if _, err := tx.Exec(ctx, `
		INSERT INTO transfer_requests (id, delivery_mode, sender_owner_id, sender_device_id,
			recipient_owner_id, recipient_device_id, expires_at, file_name, size_bytes, content_root, encryption_suite, encrypted_manifest, sender_encrypted_manifest)
		VALUES ($1,'ONLINE',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)
	`, transfer.ID, req.SenderOwnerID, req.SenderDeviceID, req.RecipientOwnerID, req.RecipientDeviceID,
		transfer.ExpiresAt, req.FileName, req.SizeBytes, req.ContentRoot, req.EncryptionSuite, req.EncryptedManifest, req.SenderEncryptedManifest); err != nil {
		return Transfer{}, err
	}
	if err := addEvent(ctx, tx, "TRANSFER_REQUESTED", transfer.ID, 1, transfer); err != nil {
		return Transfer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, err
	}
	return transfer, nil
}

func (s *Service) AddSignal(ctx context.Context, transferID string, req P2PSignalRequest) (P2PSignal, error) {
	tx, tr, err := s.beginTransferObjects(ctx, transferID)
	if err != nil {
		return P2PSignal{}, err
	}
	defer tx.Rollback(ctx)
	if !participant(tr, req.OwnerID, req.DeviceID) {
		return P2PSignal{}, ErrNotFound
	}
	if err = transferUploadAllowed(ctx, tx, tr); err != nil {
		return P2PSignal{}, err
	}
	role := "SENDER"
	if tr.RecipientOwnerID == req.OwnerID && tr.RecipientDeviceID == req.DeviceID {
		role = "RECEIVER"
	}
	if req.Role != "" && req.Role != role {
		return P2PSignal{}, ErrUnauthorized
	}
	permitted := (role == "SENDER" && (req.Channel == "direct-offer" || req.Channel == "direct-failed-sender" || req.Channel == "relay-uplink-offer")) ||
		(role == "RECEIVER" && (req.Channel == "direct-answer" || req.Channel == "direct-failed" || req.Channel == "relay-downlink-offer"))
	if !permitted || len(req.Payload) == 0 || len(req.Payload) > 4<<20 {
		return P2PSignal{}, ErrInvalid
	}
	if tr.State != "ACCEPTED" && !(tr.State == "PENDING" && role == "SENDER" && req.Channel == "relay-uplink-offer") {
		return P2PSignal{}, &StateConflictError{Resource: "transfer", Current: presentTransfer(tr, req.OwnerID, req.DeviceID)}
	}
	if req.ReplyTo < 0 {
		return P2PSignal{}, ErrInvalid
	}
	if req.Channel == "direct-answer" {
		var matches bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM peer_signals WHERE sequence=$1 AND transfer_id=$2 AND channel='direct-offer' AND sender_role='SENDER')`, req.ReplyTo, transferID).Scan(&matches); err != nil {
			return P2PSignal{}, err
		}
		if !matches {
			return P2PSignal{}, ErrInvalid
		}
	} else if req.ReplyTo != 0 {
		return P2PSignal{}, ErrInvalid
	}
	var signal P2PSignal
	err = tx.QueryRow(ctx, `INSERT INTO peer_signals(transfer_id,sender_role,channel,payload,reply_to) VALUES($1,$2,$3,$4,$5) RETURNING sequence,created_at`, transferID, role, req.Channel, req.Payload, req.ReplyTo).Scan(&signal.Sequence, &signal.CreatedAt)
	if err != nil {
		return P2PSignal{}, err
	}
	signal.Channel, signal.Payload, signal.SenderRole, signal.ReplyTo = req.Channel, req.Payload, role, req.ReplyTo
	return signal, tx.Commit(ctx)
}

func (s *Service) Signals(ctx context.Context, transferID, ownerID, deviceID string, after int64) (P2PSignalList, error) {
	tr, err := s.GetTransfer(ctx, transferID, ownerID, deviceID)
	if err != nil {
		return P2PSignalList{}, err
	}
	pendingSender := tr.State == "PENDING" && tr.SenderOwnerID == ownerID && tr.SenderDeviceID == deviceID
	if (tr.State != "ACCEPTED" && !pendingSender) || !s.now().Before(tr.ExpiresAt) {
		return P2PSignalList{}, ErrConflict
	}
	rows, err := s.db.Query(ctx, `
		SELECT sequence, sender_role, channel, payload, created_at,reply_to FROM peer_signals
		WHERE transfer_id=$1 AND sequence>$2 AND(NOT $3 OR channel IN ('relay-uplink-offer','relay-uplink-answer')) ORDER BY sequence LIMIT 200
	`, transferID, after, pendingSender)
	if err != nil {
		return P2PSignalList{}, err
	}
	defer rows.Close()
	var result P2PSignalList
	for rows.Next() {
		var signal P2PSignal
		if err := rows.Scan(&signal.Sequence, &signal.SenderRole, &signal.Channel, &signal.Payload, &signal.CreatedAt, &signal.ReplyTo); err != nil {
			return P2PSignalList{}, err
		}
		result.Signals = append(result.Signals, signal)
	}
	return result, rows.Err()
}

func (s *Service) UpdateP2PProgress(ctx context.Context, transferID string, req P2PProgressRequest) error {
	if req.RouteEpoch < 0 || (req.Route != "" && req.Route != "DIRECT" && req.Route != "RELAY") || (req.Route != "" && req.RouteEpoch == 0) || (req.Route == "" && req.RouteEpoch != 0) {
		return fmt.Errorf("%w: invalid route observation", ErrInvalid)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	transfer, err := scanTransfer(tx.QueryRow(ctx, `SELECT `+transferColumns+` FROM transfer_requests WHERE id=$1 FOR UPDATE`, transferID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if transfer.RecipientOwnerID != req.OwnerID || transfer.RecipientDeviceID != req.DeviceID || transfer.SizeBytes == nil || req.ReceivedBytes < 0 || req.ReceivedBytes > *transfer.SizeBytes {
		return fmt.Errorf("%w: invalid P2P progress", ErrInvalid)
	}
	if transfer.ContentRoot == nil {
		return ErrInvalid
	}
	if transfer.State != "ACCEPTED" {
		return &StateConflictError{Resource: "transfer", Current: presentTransfer(transfer, req.OwnerID, req.DeviceID)}
	}
	task, err := scanTask(tx.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE kind='DOWNLOAD' AND reference_id=$1 FOR UPDATE`, transferID))
	if err != nil {
		return err
	}
	if task.State != "RUNNING" {
		return &StateConflictError{Resource: "task", Current: task}
	}
	var withinDeadline bool
	if err := tx.QueryRow(ctx, `SELECT $1::timestamptz>clock_timestamp()`, transfer.ExpiresAt).Scan(&withinDeadline); err != nil {
		return err
	}
	if !withinDeadline {
		return &StateConflictError{Resource: "transfer", Current: presentTransfer(transfer, req.OwnerID, req.DeviceID)}
	}
	if req.Route != "" && req.RouteEpoch == transfer.RouteEpoch && req.Route != transfer.ActiveRoute {
		return &StateConflictError{Resource: "route", Current: presentTransfer(transfer, req.OwnerID, req.DeviceID)}
	}
	if req.ReceivedBytes > transfer.ReceivedBytes {
		if _, err := tx.Exec(ctx, `UPDATE transfer_requests SET received_bytes=$2,
			progress_revision=progress_revision+1, progress_sampled_at=now(), updated_at=now() WHERE id=$1`, transferID, req.ReceivedBytes); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `UPDATE tasks SET progress_bytes=$2,
			progress_revision=progress_revision+1, progress_sampled_at=now(), updated_at=now() WHERE id=$1`, task.ID, req.ReceivedBytes); err != nil {
			return err
		}
	}
	// A valid late ACK may advance delivery; old route telemetry cannot revert the route.
	if req.Route != "" && req.RouteEpoch > transfer.RouteEpoch {
		if _, err := tx.Exec(ctx, `UPDATE transfer_requests SET active_route=$2, route_epoch=$3 WHERE id=$1`, transferID, req.Route, req.RouteEpoch); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (s *Service) CompleteP2P(ctx context.Context, transferID string, req TransferActionRequest) (Transfer, error) {
	tx, transfer, err := s.beginTransferObjects(ctx, transferID)
	if err != nil {
		return Transfer{}, err
	}
	defer tx.Rollback(ctx)
	if transfer.RecipientOwnerID != req.OwnerID || transfer.RecipientDeviceID != req.DeviceID {
		return Transfer{}, ErrNotFound
	}
	if transfer.State == "COMPLETED" {
		return presentTransfer(transfer, req.OwnerID, req.DeviceID), tx.Commit(ctx)
	}
	if transfer.State != "ACCEPTED" || transfer.Version != req.ExpectedVersion {
		return Transfer{}, &StateConflictError{Resource: "transfer", Current: presentTransfer(transfer, req.OwnerID, req.DeviceID)}
	}
	if transfer.SizeBytes == nil || transfer.ReceivedBytes != *transfer.SizeBytes {
		return Transfer{}, &StateConflictError{Resource: "transfer", Current: presentTransfer(transfer, req.OwnerID, req.DeviceID)}
	}
	task, err := scanTask(tx.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE kind='DOWNLOAD' AND reference_id=$1 FOR UPDATE`, transferID))
	if err != nil {
		return Transfer{}, err
	}
	if task.State != "RUNNING" || task.ProgressBytes != task.TotalBytes {
		return Transfer{}, &StateConflictError{Resource: "task", Current: task}
	}
	var withinDeadline bool
	if err := tx.QueryRow(ctx, `SELECT $1::timestamptz>clock_timestamp()`, transfer.ExpiresAt).Scan(&withinDeadline); err != nil {
		return Transfer{}, err
	}
	if !withinDeadline {
		return Transfer{}, ErrExpired
	}
	transfer.State = "COMPLETED"
	transfer.Version++
	transfer.UpdatedAt = s.now()
	if _, err := tx.Exec(ctx, `UPDATE transfer_requests SET state='COMPLETED', version=$2, updated_at=now(), responded_at=now() WHERE id=$1`, transferID, transfer.Version); err != nil {
		return Transfer{}, err
	}
	if _, err := tx.Exec(ctx, `UPDATE tasks SET state='COMPLETED', progress_bytes=total_bytes, version=version+1, completed_at=now(), updated_at=now() WHERE kind='DOWNLOAD' AND reference_id=$1`, transferID); err != nil {
		return Transfer{}, err
	}
	if transfer.EntryID != nil {
		if err := recordP2PReceipt(ctx, tx, transfer, *transfer.EntryID); err != nil {
			return Transfer{}, err
		}
	}
	if err := grantRestore(ctx, tx, transfer); err != nil {
		return Transfer{}, err
	}
	// Finishing delivery removes the request deadline from an already-open spool pin;
	// it does not extend its last-activity retention clock.
	if _, err := tx.Exec(ctx, `UPDATE upload_sessions SET last_activity_at=last_activity_at WHERE transfer_id=$1 AND state='OPEN'`, transferID); err != nil {
		return Transfer{}, err
	}
	if err := addEvent(ctx, tx, "TRANSFER_COMPLETED", transferID, transfer.Version, transfer); err != nil {
		return Transfer{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Transfer{}, err
	}
	return presentTransfer(transfer, req.OwnerID, req.DeviceID), nil
}
