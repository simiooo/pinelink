package service

import (
	"context"
	"errors"
)

func (m *Maintenance) revokeInactive(ctx context.Context) error {
	rows, err := m.db.Query(ctx, `SELECT tr.id FROM transfer_requests tr WHERE tr.state IN ('PENDING','ACCEPTED') AND EXISTS(
        SELECT 1 FROM devices d WHERE d.status='REVOKED' AND ((d.owner_id=tr.sender_owner_id AND d.device_id=tr.sender_device_id) OR(d.owner_id=tr.recipient_owner_id AND d.device_id=tr.recipient_device_id))) LIMIT 50`)
	if err != nil {
		return err
	}
	ids, err := scanIDs(rows)
	if err != nil {
		return err
	}
	var errs []error
	for _, id := range ids {
		if err := m.revokeInactiveTransfer(ctx, id); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (m *Maintenance) revokeInactiveTransfer(ctx context.Context, id string) error {
	tx, tr, err := New(m.db, m.blobs).beginTransferObjects(ctx, id)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if tr.State != "PENDING" && tr.State != "ACCEPTED" {
		return nil
	}
	var revoked bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM devices WHERE status='REVOKED' AND((owner_id=$1 AND device_id=$2) OR(owner_id=$3 AND device_id=$4)))`, tr.SenderOwnerID, tr.SenderDeviceID, tr.RecipientOwnerID, tr.RecipientDeviceID).Scan(&revoked); err != nil {
		return err
	}
	if !revoked {
		return nil
	}
	tr.State = "CANCELLED"
	tr.Version++
	tr.RevocationEpoch++
	if _, err = tx.Exec(ctx, `UPDATE transfer_requests SET state='CANCELLED',version=version+1,revocation_epoch=revocation_epoch+1,updated_at=now(),responded_at=now(),failure_reason='DEVICE_REVOKED' WHERE id=$1`, id); err != nil {
		return err
	}
	if err = releaseTransferResources(ctx, tx, tr); err != nil {
		return err
	}
	if err = addEvent(ctx, tx, "TRANSFER_CANCELLED", id, tr.Version, tr); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
