package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const taskColumns = `id, kind, reference_id, owner_id, device_id, state, progress_bytes,
	total_bytes, version, error_code, error_message, created_at, updated_at, completed_at,
	progress_revision, progress_sampled_at`

func createTask(ctx context.Context, tx pgx.Tx, kind, referenceID, ownerID, deviceID string, total int64) (string, error) {
	id := uuid.NewString()
	_, err := tx.Exec(ctx, `
		INSERT INTO tasks (id, kind, reference_id, owner_id, device_id, state, total_bytes)
		VALUES ($1,$2,$3,$4,$5,'RUNNING',$6)
	`, id, kind, referenceID, ownerID, deviceID, total)
	return id, err
}

func completeTask(ctx context.Context, tx pgx.Tx, kind, referenceID string) error {
	_, err := tx.Exec(ctx, `
		UPDATE tasks SET state='COMPLETED', progress_bytes=total_bytes, version=version+1,
			updated_at=now(), completed_at=now(), error_code=NULL, error_message=NULL
		WHERE kind=$1 AND reference_id=$2 AND state NOT IN ('CANCELLED', 'DELETED')
	`, kind, referenceID)
	return err
}

func (s *Service) GetTask(ctx context.Context, id, ownerID, deviceID string) (Task, error) {
	task, err := scanTask(s.db.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE (id=$1 OR reference_id=$1) AND owner_id=$2 AND device_id=$3`, id, ownerID, deviceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	return task, err
}

func (s *Service) ListTasks(ctx context.Context, ownerID, deviceID, kind, state string, includeDeleted bool, before *time.Time, limit int) (TaskList, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	query := `SELECT ` + taskColumns + ` FROM tasks WHERE owner_id=$1 AND device_id=$2`
	args := []any{ownerID, deviceID}
	if kind != "" {
		args = append(args, kind)
		query += fmt.Sprintf(" AND kind=$%d", len(args))
	}
	if state != "" {
		args = append(args, state)
		query += fmt.Sprintf(" AND state=$%d", len(args))
	}
	if !includeDeleted {
		query += " AND state<>'DELETED'"
	}
	if before != nil {
		args = append(args, *before)
		query += fmt.Sprintf(" AND updated_at<$%d", len(args))
	}
	args = append(args, limit+1)
	query += fmt.Sprintf(" ORDER BY updated_at DESC LIMIT $%d", len(args))
	rows, err := s.db.Query(ctx, query, args...)
	if err != nil {
		return TaskList{}, err
	}
	defer rows.Close()
	var result TaskList
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			return TaskList{}, err
		}
		if len(result.Tasks) == limit {
			result.NextBefore = &task.UpdatedAt
			break
		}
		result.Tasks = append(result.Tasks, task)
	}
	return result, rows.Err()
}

func (s *Service) ControlTask(ctx context.Context, id, action string, req TaskActionRequest) (Task, error) {
	observed, err := s.GetTask(ctx, id, req.OwnerID, req.DeviceID)
	if err != nil {
		return Task{}, err
	}
	var tx pgx.Tx
	var upload uploadObject
	if observed.Kind == "UPLOAD" {
		tx, upload, err = s.beginUploadObject(ctx, observed.ReferenceID, false)
	} else {
		tx, err = s.db.Begin(ctx)
	}
	if err != nil {
		return Task{}, err
	}
	defer tx.Rollback(ctx)
	task, err := scanTask(tx.QueryRow(ctx, `SELECT `+taskColumns+` FROM tasks WHERE (id=$1 OR reference_id=$1) AND owner_id=$2 AND device_id=$3 FOR UPDATE`, id, req.OwnerID, req.DeviceID))
	if errors.Is(err, pgx.ErrNoRows) {
		return Task{}, ErrNotFound
	}
	if err != nil {
		return Task{}, err
	}
	desired := map[string]string{"pause": "PAUSED", "resume": "RUNNING", "cancel": "CANCELLED", "delete": "DELETED", "fail": "FAILED"}[action]
	if desired == "" {
		return Task{}, fmt.Errorf("%w: unknown task action", ErrInvalid)
	}
	if task.State == desired {
		return task, tx.Commit(ctx)
	}
	if task.Version != req.ExpectedVersion {
		return Task{}, &StateConflictError{Resource: "task", Current: task}
	}
	valid := (desired == "PAUSED" && (task.State == "RUNNING" || task.State == "QUEUED")) ||
		(desired == "RUNNING" && task.State == "PAUSED") ||
		(desired == "CANCELLED" && task.State != "COMPLETED" && task.State != "DELETED") ||
		(desired == "DELETED" && task.State != "RUNNING") ||
		(desired == "FAILED" && task.State != "COMPLETED" && task.State != "DELETED")
	if !valid {
		return Task{}, &StateConflictError{Resource: "task", Current: task}
	}
	task.State = desired
	task.Version++
	task.UpdatedAt = s.now()
	if desired == "CANCELLED" || desired == "DELETED" || desired == "FAILED" {
		task.CompletedAt = &task.UpdatedAt
		code := "TASK_" + desired
		task.ErrorCode = &code
	}
	if desired == "FAILED" {
		task.ErrorMessage = &req.Reason
	}
	if _, err := tx.Exec(ctx, `
		UPDATE tasks SET state=$2, version=$3, updated_at=now(), completed_at=$4, error_code=$5, error_message=$6 WHERE id=$1
	`, task.ID, task.State, task.Version, task.CompletedAt, task.ErrorCode, task.ErrorMessage); err != nil {
		return Task{}, err
	}
	if task.Kind == "UPLOAD" && upload.State == "OPEN" && (desired == "CANCELLED" || desired == "DELETED" || desired == "FAILED") {
		state := "CANCELLED"
		if desired == "FAILED" {
			state = "FAILED"
		}
		if _, err = tx.Exec(ctx, `UPDATE upload_sessions SET state=$2 WHERE id=$1`, task.ReferenceID, state); err != nil {
			return Task{}, err
		}
		if _, err = tx.Exec(ctx, `UPDATE file_versions SET gc_after=now() WHERE id=$1`, upload.VersionID); err != nil {
			return Task{}, err
		}
	}
	if task.Kind == "DOWNLOAD" && (desired == "CANCELLED" || desired == "DELETED") {
		if _, err := tx.Exec(ctx, `UPDATE pull_sessions SET state='FAILED', error_code=$2, version=version+1, updated_at=now() WHERE id=$1 AND state='PULLING'`, task.ReferenceID, task.ErrorCode); err != nil {
			return Task{}, err
		}
	}
	if err := addEvent(ctx, tx, "TASK_STATE_CHANGED", task.ID, task.Version, task); err != nil {
		return Task{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Task{}, err
	}
	return task, nil
}

func (s *Service) ReportTaskProgress(ctx context.Context, id string, req TaskProgressRequest) (Task, error) {
	if req.ProgressBytes < 0 {
		return Task{}, fmt.Errorf("%w: negative progress", ErrInvalid)
	}
	current, err := s.GetTask(ctx, id, req.OwnerID, req.DeviceID)
	if err != nil {
		return Task{}, err
	}
	// Upload progress comes only from committed chunks, never a client offset.
	if current.Kind == "UPLOAD" {
		if current.State != "RUNNING" || req.ProgressBytes > current.ProgressBytes {
			return Task{}, &StateConflictError{Resource: "task", Current: current}
		}
		return current, nil
	}
	// The legacy task endpoint must not bypass the P2P transfer's consent/terminal checks.
	var p2p bool
	if current.Kind == "DOWNLOAD" {
		if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM transfer_requests WHERE id=$1 AND content_root IS NOT NULL)`, current.ReferenceID).Scan(&p2p); err != nil {
			return Task{}, err
		}
	}
	if p2p {
		if err := s.UpdateP2PProgress(ctx, current.ReferenceID, P2PProgressRequest{OwnerID: req.OwnerID, DeviceID: req.DeviceID, ReceivedBytes: req.ProgressBytes}); err != nil {
			return Task{}, err
		}
		return s.GetTask(ctx, current.ID, req.OwnerID, req.DeviceID)
	}
	result, err := s.db.Exec(ctx, `
		UPDATE tasks SET progress_bytes=$4, progress_revision=progress_revision+1,
			progress_sampled_at=now(), updated_at=now()
		WHERE id=$1 AND owner_id=$2 AND device_id=$3 AND state='RUNNING'
			AND $4<=total_bytes AND $4>progress_bytes
	`, current.ID, req.OwnerID, req.DeviceID, req.ProgressBytes)
	if err != nil {
		return Task{}, err
	}
	if result.RowsAffected() == 0 {
		current, getErr := s.GetTask(ctx, id, req.OwnerID, req.DeviceID)
		if getErr != nil {
			return Task{}, getErr
		}
		if current.State == "RUNNING" && req.ProgressBytes <= current.TotalBytes {
			return current, nil
		}
		return Task{}, &StateConflictError{Resource: "task", Current: current}
	}
	return s.GetTask(ctx, id, req.OwnerID, req.DeviceID)
}

func scanTask(row rowScanner) (Task, error) {
	var task Task
	err := row.Scan(&task.ID, &task.Kind, &task.ReferenceID, &task.OwnerID, &task.DeviceID,
		&task.State, &task.ProgressBytes, &task.TotalBytes, &task.Version, &task.ErrorCode,
		&task.ErrorMessage, &task.CreatedAt, &task.UpdatedAt, &task.CompletedAt,
		&task.ProgressRevision, &task.ProgressSampledAt)
	return task, err
}
