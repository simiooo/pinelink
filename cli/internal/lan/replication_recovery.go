package lan

import (
	"context"
	"encoding/json"
	"errors"
	"sync"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

func (d *stateDirectory) outboundRecords() ([]sentRecord, error) {
	keys, err := d.keys("sent")
	if err != nil {
		return nil, err
	}
	result := []sentRecord{}
	for _, key := range keys {
		var sent sentRecord
		if err = readJSON(d, key, &sent); err != nil {
			return nil, err
		}
		if !sent.Status.terminal() {
			result = append(result, sent)
		}
	}
	return result, nil
}

// Byte recovery shares the peer's connection backoff instead of probing a
// disconnected device again for every pending transfer on every loop.
func (d *stateDirectory) peerReachable(root string) bool {
	var ready bool
	_ = d.db.QueryRow("SELECT enabled=1 AND last_error='' AND last_success_at IS NOT NULL FROM replica_targets WHERE target=?", "peer:"+root).Scan(&ready)
	return ready
}

// History application remains independent of byte delivery. This recovery step
// can be repeated after any crash between inbox commit and local reconciliation.
func (d *stateDirectory) reconcileTransfers(ctx context.Context, identity *secure.Identity) error {
	records, err := d.outboundRecords()
	if err != nil {
		return err
	}
	processed := 0
	for _, sent := range records {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if processed >= 8 {
			break
		}
		processed++
		var body []byte
		if d.db.QueryRow(`SELECT body FROM replica_history WHERE authority_root=? AND aggregate_id=? AND source_root=? AND kind='TRANSFER_CHANGED'`, sent.Peer.ID, sent.Offer.ID, sent.Peer.ID).Scan(&body) == nil {
			var fact transferFact
			if json.Unmarshal(body, &fact) != nil || fact.Version < sent.Status.Version {
				continue
			}
			if fact.Version > sent.Status.Version || fact.State != sent.Status.State {
				next := sent.Status
				next.State = fact.State
				next.Version = fact.Version
				next.Receipt = fact.Receipt
				next.Error = fact.Error
				if fact.State == "COMPLETED" {
					if fact.Receipt == nil || fact.Receipt.Verify(sent.Peer, sent.Offer, identity.LocalID()) != nil || fact.Receipt.PlainRoot != sent.PlainRoot || fact.Receipt.PlainSize != sent.PlainSize {
						return ErrUnauthorized
					}
					next.Acknowledged = next.Size
				}
				sent.Status = next
				if err = writeJSON(d, sentName(sent.Offer.ID), sent); err != nil && !errors.Is(err, ErrConflict) {
					return err
				}
				if next.terminal() {
					_ = d.Remove(sent.CipherName)
					continue
				}
			}
		}
		if sent.Status.State == "PREPARING" {
			if !d.peerReachable(sent.Peer.ID) {
				continue
			}
			peer, err := ReadContact(d.Name(), sent.Peer.ID)
			if err != nil {
				continue
			}
			client, err := d.resolvePeer(ctx, identity, peer)
			if err != nil {
				continue
			}
			request, _ := json.Marshal(sent.Offer)
			var transfer Transfer
			err = client.request(ctx, "POST", "transfers", request, &transfer)
			client.Close()
			if err != nil {
				continue
			}
			sent.Status = sendSnapshot(transfer, peer.ID)
			if err = writeJSON(d, sentName(sent.Offer.ID), sent); err != nil && !errors.Is(err, ErrConflict) {
				return err
			}
		}
	}
	return nil
}

type recoveryJobs struct {
	active map[string]bool
	mu     sync.Mutex
	group  sync.WaitGroup
}

func (j *recoveryJobs) launch(ctx context.Context, d *stateDirectory, identity *secure.Identity) {
	records, err := d.outboundRecords()
	if err != nil {
		return
	}
	for _, sent := range records {
		if !sent.AutoSend || (sent.Status.State != "ACCEPTED" && sent.Status.State != "VERIFYING") {
			continue
		}
		if !d.peerReachable(sent.Peer.ID) {
			continue
		}
		j.mu.Lock()
		if j.active == nil {
			j.active = map[string]bool{}
		}
		if len(j.active) >= 2 || j.active[sent.Offer.ID] {
			j.mu.Unlock()
			continue
		}
		j.active[sent.Offer.ID] = true
		j.group.Add(1)
		j.mu.Unlock()
		sent := sent
		go func() {
			defer j.group.Done()
			defer func() { j.mu.Lock(); delete(j.active, sent.Offer.ID); j.mu.Unlock() }()
			peer, err := ReadContact(d.Name(), sent.Peer.ID)
			if err != nil {
				return
			}
			client, err := NewClient(identity, peer, d.Name())
			if err != nil {
				return
			}
			defer client.Close()
			attempt, cancel := context.WithCancel(ctx)
			defer cancel()
			_, _ = client.Send(attempt, sent.Offer.ID, func(t Transfer) {
				if t.State == "PAUSED" {
					cancel()
				}
			})
		}()
	}
}
