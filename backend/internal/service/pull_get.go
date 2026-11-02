package service

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

func (s *Service) GetPull(ctx context.Context, pullID, ownerID, deviceID string) (Pull, error) {
	var pull Pull
	err := s.db.QueryRow(ctx, `
        SELECT p.id,t.id,COALESCE(p.transfer_id::text,''),p.entry_id,p.device_id,e.file_name,v.content_root,v.size_bytes
        FROM pull_sessions p JOIN tasks t ON t.kind='DOWNLOAD' AND t.reference_id=p.id
        JOIN file_entries e ON e.id=p.entry_id JOIN file_versions v ON v.id=e.version_id
        WHERE p.id=$1 AND t.owner_id=$2 AND t.device_id=$3`, pullID, ownerID, deviceID).Scan(&pull.ID, &pull.TaskID, &pull.TransferID, &pull.EntryID, &pull.DeviceID, &pull.FileName, &pull.ContentRoot, &pull.SizeBytes)
	if errors.Is(err, pgx.ErrNoRows) {
		return Pull{}, ErrNotFound
	}
	if err != nil {
		return Pull{}, err
	}
	if err = fileReadAccess(ctx, s.db, pull.EntryID, ownerID, deviceID, pull.TransferID); err != nil {
		return Pull{}, err
	}
	return pull, nil
}
