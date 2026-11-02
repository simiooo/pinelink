package service

import (
	"context"
	"time"
)

type TaskProjection struct {
	State            string     `json:"state"`
	ProgressBytes    int64      `json:"progress_bytes"`
	TotalBytes       int64      `json:"total_bytes"`
	Version          int64      `json:"version"`
	ErrorCode        *string    `json:"error_code,omitempty"`
	ProgressRevision int64      `json:"progress_revision"`
	SampledAt        *time.Time `json:"sampled_at"`
}

func (s *Service) GetTaskProjection(ctx context.Context, id, ownerID, deviceID string) (TaskProjection, error) {
	task, err := s.GetTask(ctx, id, ownerID, deviceID)
	if err != nil {
		return TaskProjection{}, err
	}
	projection := TaskProjection{ProgressBytes: task.ProgressBytes, TotalBytes: task.TotalBytes, Version: task.Version, ErrorCode: task.ErrorCode}
	projection.ProgressRevision, projection.SampledAt = task.ProgressRevision, task.ProgressSampledAt
	switch task.State {
	case "QUEUED":
		projection.State = "PREPARING"
	case "RUNNING":
		projection.State = "SENDING"
		if task.Kind == "DOWNLOAD" {
			projection.State = "RECEIVING"
		}
		if task.TotalBytes > 0 && task.ProgressBytes == task.TotalBytes {
			projection.State = "VERIFYING"
		}
	case "PAUSED":
		projection.State = "PAUSED"
	case "COMPLETED":
		if task.Kind == "UPLOAD" {
			projection.State = "STORED"
		} else {
			projection.State = "DELIVERED"
		}
	case "CANCELLED", "DELETED":
		projection.State = "CANCELLED"
	case "FAILED":
		projection.State = "FAILED"
	default:
		projection.State = "RECOVERING"
	}
	return projection, nil
}
