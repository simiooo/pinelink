package lan

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/simiooo/pinelink/cli/internal/secure"
	rp "github.com/simiooo/pinelink/protocol/replication"
)

func (d *stateDirectory) publication(identity *secure.Identity, change Change) (*rp.Envelope, error) {
	if change.Kind != "SENT_CHANGED" && change.Kind != "TRANSFER_CHANGED" {
		return nil, nil
	}
	var existing []byte
	err := d.db.QueryRow("SELECT body FROM replica_publications WHERE event_id=?", change.EventID).Scan(&existing)
	if err == nil {
		var e rp.Envelope
		err = json.Unmarshal(existing, &e)
		return &e, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	var fact transferFact
	if json.Unmarshal(change.Payload, &fact) != nil || fact.ID != change.AggregateID || !rp.ValidRoot(fact.PeerID) {
		return nil, rp.ErrInvalid
	}
	recipients := []string{identity.LocalID()}
	keys := []string{identity.PublicKey()}
	var peer Peer
	trusted := readJSON(d, "peer-"+fact.PeerID+".json", &peer) == nil && !peer.Revoked && peer.validate() == nil
	if trusted {
		recipients = append(recipients, peer.ID)
		keys = append(keys, peer.EncryptionKey)
	}
	slices.Sort(recipients)
	ciphertext, err := secure.SealHistory(change.Payload, keys)
	if err != nil {
		return nil, err
	}
	e := rp.Envelope{Version: rp.Version, EventID: change.EventID, SourceRoot: identity.LocalID(), StreamID: d.streamID, Sequence: change.Sequence, AggregateID: fact.ID, AggregateVersion: fact.Version, Authority: fact.Authority, AuthorityRoot: fact.AuthorityID, Kind: change.Kind, PeerRoot: fact.PeerID, Recipients: recipients, SigningKey: identity.SigningPublicKey(), Ciphertext: ciphertext}
	e.Signature = identity.Sign(e.Proof())
	if e.Validate() != nil {
		return nil, rp.ErrInvalid
	}
	body, _ := json.Marshal(e)
	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if trusted {
		var current []byte
		if err = tx.QueryRow("SELECT body FROM records WHERE record_key=?", "peer-"+peer.ID+".json").Scan(&current); err != nil {
			return nil, err
		}
		var now Peer
		if json.Unmarshal(current, &now) != nil || now.Revoked || now.SigningKey != peer.SigningKey || now.EncryptionKey != peer.EncryptionKey {
			return nil, ErrConflict
		}
	}
	if _, err = tx.Exec("INSERT OR IGNORE INTO replica_publications(event_id,sequence,body,payload_hash) VALUES(?,?,?,?)", e.EventID, e.Sequence, body, rp.Hash(body)); err != nil {
		return nil, err
	}
	if err = tx.QueryRow("SELECT body FROM replica_publications WHERE event_id=?", e.EventID).Scan(&existing); err != nil {
		return nil, err
	}
	if _, err = tx.Exec("UPDATE sync_outbox SET binding_state=CASE WHEN binding_state='UNBOUND' THEN 'PREPARED' ELSE binding_state END WHERE event_id=?", e.EventID); err != nil {
		return nil, err
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if err = json.Unmarshal(existing, &e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (d *stateDirectory) uploadBatch(identity *secure.Identity, after int64, peer string) (rp.Upload, error) {
	b := rp.Upload{SourceRoot: d.deviceID, StreamID: d.streamID, After: after, Through: after, Events: []rp.Envelope{}}
	page, err := d.changes(after, rp.MaxBatch, d.streamID)
	if err != nil {
		return b, err
	}
	for _, change := range page.Items {
		e, err := d.publication(identity, change)
		if err != nil {
			return b, err
		}
		if e != nil && (peer == "" || slices.Contains(e.Recipients, peer)) {
			b.Events = append(b.Events, *e)
		}
		b.Through = change.Sequence
	}
	return b, nil
}

func (d *stateDirectory) replicaInfo(identity *secure.Identity, nonce string) (rp.Info, error) {
	if len(nonce) != 43 {
		return rp.Info{}, ErrConflict
	}
	i := rp.Info{Version: rp.Version, Epoch: d.streamID, PublicKey: identity.SigningPublicKey(), UnixTime: time.Now().Unix(), Nonce: nonce}
	i.Signature = identity.Sign(i.Proof())
	return i, nil
}

func (d *stateDirectory) replicaPage(identity *secure.Identity, consumer, epoch string, after int64, snapshot bool, upper int64, key string) (rp.Page, error) {
	p := rp.Page{Root: consumer, Epoch: d.streamID, After: after, Through: after, Items: []rp.Delivery{}, Snapshot: snapshot}
	if after < 0 || len(key) > 300 || (epoch != "" && epoch != d.streamID) {
		return p, ErrConflict
	}
	if err := d.db.QueryRow("SELECT COALESCE(MAX(sequence),0) FROM change_log").Scan(&p.Upper); err != nil {
		return p, err
	}
	if after > p.Upper {
		return p, ErrConflict
	}
	if snapshot && upper >= 0 {
		if upper > p.Upper {
			return p, ErrConflict
		}
		p.Upper = upper
	}
	if !snapshot {
		batch, err := d.uploadBatch(identity, after, consumer)
		if err != nil {
			return p, err
		}
		p.Through = min(batch.Through, p.Upper)
		for _, e := range batch.Events {
			if e.Sequence <= p.Upper {
				p.Items = append(p.Items, rp.Delivery{Sequence: e.Sequence, Event: e})
			}
		}
		p.More = p.Through < p.Upper
	} else {
		prefix := d.deviceID + "|" + d.streamID + "|"
		rows, err := d.db.Query(`WITH latest AS (
SELECT MAX(sequence) AS sequence FROM change_log WHERE sequence<=? AND kind IN ('SENT_CHANGED','TRANSFER_CHANGED') AND json_extract(payload,'$.peer_id')=? GROUP BY kind,aggregate_id)
SELECT c.sequence,c.event_id,c.kind,c.aggregate_id,c.payload,c.created_at FROM change_log c JOIN latest l ON c.sequence=l.sequence
WHERE ?||c.kind||'|'||c.aggregate_id>? ORDER BY c.kind,c.aggregate_id LIMIT ?`, p.Upper, consumer, prefix, key, rp.MaxBatch+1)
		if err != nil {
			return p, err
		}
		var changes []Change
		for rows.Next() {
			var c Change
			if err = rows.Scan(&c.Sequence, &c.EventID, &c.Kind, &c.AggregateID, &c.Payload, &c.CreatedAt); err != nil {
				rows.Close()
				return p, err
			}
			changes = append(changes, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return p, err
		}
		p.More = len(changes) > rp.MaxBatch
		if p.More {
			changes = changes[:rp.MaxBatch]
		}
		for _, c := range changes {
			e, err := d.publication(identity, c)
			if err != nil {
				return p, err
			}
			if e != nil && slices.Contains(e.Recipients, consumer) {
				p.Items = append(p.Items, rp.Delivery{Sequence: e.Sequence, Event: *e})
			}
		}
		if p.More {
			last := changes[len(changes)-1]
			p.NextKey = prefix + last.Kind + "|" + last.AggregateID
		}
		p.Through = p.Upper
	}
	p.Signature = identity.Sign(p.Proof())
	return p, nil
}

func verifyReplicaFact(identity *secure.Identity, e rp.Envelope, plaintext []byte, peerKey func(string) (Peer, bool)) (transferFact, error) {
	var fact transferFact
	if json.Unmarshal(plaintext, &fact) != nil || fact.ID != e.AggregateID || fact.Version != e.AggregateVersion || fact.Authority != e.Authority || fact.AuthorityID != e.AuthorityRoot || fact.PeerID != e.PeerRoot || fact.PlainSize < 0 || len(fact.FileName) > 512 {
		return fact, rp.ErrInvalid
	}
	if (e.Kind == "SENT_CHANGED" && fact.Direction != "SEND") || (e.Kind == "TRANSFER_CHANGED" && fact.Direction != "RECEIVE") {
		return fact, rp.ErrInvalid
	}
	switch fact.State {
	case "PREPARING", "PENDING", "ACCEPTED", "PAUSED", "VERIFYING", "COMPLETED", "REJECTED", "CANCELLED", "EXPIRED", "FAILED":
	default:
		return fact, rp.ErrInvalid
	}
	if fact.State == "COMPLETED" {
		if fact.Receipt == nil {
			return fact, rp.ErrInvalid
		}
		var recipient Peer
		if fact.AuthorityID == identity.LocalID() {
			recipient = Describe(identity, "This device", "")
		} else {
			var ok bool
			recipient, ok = peerKey(fact.AuthorityID)
			if !ok {
				return fact, ErrForbidden
			}
		}
		sender := e.SourceRoot
		if e.Kind == "TRANSFER_CHANGED" {
			sender = fact.PeerID
		}
		if fact.Receipt.Verify(recipient, Offer{ID: fact.ID, CipherRoot: fact.CipherRoot}, sender) != nil || fact.Receipt.PlainSize != fact.PlainSize {
			return fact, rp.ErrInvalid
		}
	} else if fact.Receipt != nil {
		return fact, rp.ErrInvalid
	}
	return fact, nil
}

// Apply metadata and its consumed position together. Runtime transfer ledgers
// remain local authority; replicated history cannot grant object access.
func (d *stateDirectory) applyReplicaPage(ctx context.Context, identity *secure.Identity, target string, page rp.Page, pin string, incoming ...rp.Upload) (int, error) {
	if page.Validate(pin, identity.LocalID()) != nil {
		return 0, rp.ErrInvalid
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var uploaded int64
	var currentEpoch string
	var currentCursor int64
	var currentBootstrap bool
	if len(incoming) == 0 {
		if err = tx.QueryRowContext(ctx, "SELECT pulled_epoch,pulled_sequence,bootstrapped FROM replica_targets WHERE target=?", target).Scan(&currentEpoch, &currentCursor, &currentBootstrap); err != nil {
			return 0, err
		}
		if currentEpoch != "" && currentEpoch != page.Epoch {
			return 0, ErrConflict
		}
	}
	if len(incoming) > 0 {
		b := incoming[0]
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO replica_peer_uploads(source_root,stream_id) VALUES(?,?)", b.SourceRoot, b.StreamID); err != nil {
			return 0, err
		}
		if err = tx.QueryRowContext(ctx, "SELECT sequence FROM replica_peer_uploads WHERE source_root=? AND stream_id=?", b.SourceRoot, b.StreamID).Scan(&uploaded); err != nil {
			return 0, err
		}
		if b.After > uploaded {
			return 0, ErrConflict
		}
		for _, e := range b.Events {
			if e.Sequence <= uploaded {
				var hash string
				body, _ := json.Marshal(e)
				if err = tx.QueryRowContext(ctx, "SELECT payload_hash FROM replica_inbox WHERE source_root=? AND stream_id=? AND event_id=?", e.SourceRoot, e.StreamID, e.EventID).Scan(&hash); err != nil || hash != rp.Hash(body) {
					return 0, ErrConflict
				}
			}
		}
	}
	lookup := func(root string) (Peer, bool) {
		var body []byte
		if tx.QueryRowContext(ctx, "SELECT body FROM records WHERE record_key=?", "peer-"+root+".json").Scan(&body) != nil {
			return Peer{}, false
		}
		var peer Peer
		if json.Unmarshal(body, &peer) != nil || peer.validate() != nil {
			return Peer{}, false
		}
		return peer, true
	}
	count := 0
	for _, delivery := range page.Items {
		e := delivery.Event
		body, _ := json.Marshal(e)
		hash := rp.Hash(body)
		var old, oldStatus string
		err = tx.QueryRowContext(ctx, "SELECT payload_hash,status FROM replica_inbox WHERE source_root=? AND stream_id=? AND event_id=?", e.SourceRoot, e.StreamID, e.EventID).Scan(&old, &oldStatus)
		wasIgnored := false
		if err == nil {
			if old != hash {
				return 0, ErrConflict
			}
			if oldStatus == "APPLIED" {
				continue
			}
			wasIgnored = true
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		status := "APPLIED"
		var plaintext []byte
		if e.Authority == "SERVER" {
			if strings.HasPrefix(target, "peer:") || e.SigningKey != pin {
				return 0, ErrForbidden
			}
			plaintext = e.ServerPayload
		} else {
			if e.SourceRoot == identity.LocalID() {
				if e.SigningKey != identity.SigningPublicKey() {
					return 0, ErrForbidden
				}
			} else {
				peer, ok := lookup(e.SourceRoot)
				if !ok || peer.Revoked || peer.SigningKey != e.SigningKey {
					status = "IGNORED_UNTRUSTED"
				}
			}
			if status == "APPLIED" {
				plaintext, err = identity.OpenHistory(e.Ciphertext)
				if err != nil {
					return 0, err
				}
				if _, err = verifyReplicaFact(identity, e, plaintext, lookup); err != nil {
					return 0, err
				}
			}
		}
		if wasIgnored {
			_, err = tx.ExecContext(ctx, "UPDATE replica_inbox SET status=? WHERE source_root=? AND stream_id=? AND event_id=?", status, e.SourceRoot, e.StreamID, e.EventID)
		} else {
			_, err = tx.ExecContext(ctx, "INSERT INTO replica_inbox(source_root,stream_id,event_id,source_sequence,payload_hash,status,body) VALUES(?,?,?,?,?,?,?)", e.SourceRoot, e.StreamID, e.EventID, e.Sequence, hash, status, body)
		}
		if err != nil {
			return 0, err
		}
		if status == "APPLIED" {
			var oldVersion int64
			readErr := tx.QueryRowContext(ctx, "SELECT version FROM replica_history WHERE authority_root=? AND aggregate_id=? AND source_root=? AND kind=?", e.AuthorityRoot, e.AggregateID, e.SourceRoot, e.Kind).Scan(&oldVersion)
			if readErr != nil && !errors.Is(readErr, sql.ErrNoRows) {
				return 0, readErr
			}
			if errors.Is(readErr, sql.ErrNoRows) || e.AggregateVersion > oldVersion {
				_, err = tx.ExecContext(ctx, `INSERT INTO replica_history(authority_root,aggregate_id,source_root,kind,version,stream_id,sequence,body,event_id) VALUES(?,?,?,?,?,?,?,?,?)
ON CONFLICT(authority_root,aggregate_id,source_root,kind) DO UPDATE SET version=excluded.version,stream_id=excluded.stream_id,sequence=excluded.sequence,body=excluded.body,event_id=excluded.event_id`, e.AuthorityRoot, e.AggregateID, e.SourceRoot, e.Kind, e.AggregateVersion, e.StreamID, e.Sequence, plaintext, e.EventID)
				if err != nil {
					return 0, err
				}
			}
			count++
		}
	}
	if len(incoming) > 0 {
		b := incoming[0]
		_, err = tx.ExecContext(ctx, "UPDATE replica_peer_uploads SET sequence=? WHERE source_root=? AND stream_id=?", max(uploaded, b.Through), b.SourceRoot, b.StreamID)
	} else if page.Snapshot && currentBootstrap && currentEpoch == page.Epoch {
		// A concurrent consumer already finished this snapshot.
	} else if page.Snapshot && page.More {
		_, err = tx.ExecContext(ctx, "UPDATE replica_targets SET pulled_epoch=?,snapshot_upper=?,snapshot_key=? WHERE target=?", page.Epoch, page.Upper, page.NextKey, target)
	} else {
		cursor := page.Through
		if currentEpoch == page.Epoch {
			cursor = max(cursor, currentCursor)
		}
		_, err = tx.ExecContext(ctx, "UPDATE replica_targets SET pulled_epoch=?,pulled_sequence=?,bootstrapped=1,snapshot_upper=-1,snapshot_key='' WHERE target=?", page.Epoch, cursor, target)
		if err == nil {
			issuer, _ := rp.Root(pin)
			_, err = tx.ExecContext(ctx, `INSERT INTO sync_cursors(destination,source_device_id,stream_id,sequence) VALUES(?,?,?,?)
ON CONFLICT(destination,source_device_id,stream_id) DO UPDATE SET sequence=MAX(sequence,excluded.sequence)`, target, issuer, page.Epoch, page.Through)
		}
	}
	if err != nil {
		return 0, err
	}
	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return count, nil
}

type ReplicatedHistory struct {
	AuthorityRoot string          `json:"authority_root"`
	AggregateID   string          `json:"aggregate_id"`
	SourceRoot    string          `json:"source_root"`
	Kind          string          `json:"kind"`
	Version       int64           `json:"version"`
	Fact          json.RawMessage `json:"fact"`
}

func ReplicaHistory(identity *secure.Identity, directory string) ([]ReplicatedHistory, error) {
	d, err := openStateDirectory(directory, identity.LocalID())
	if err != nil {
		return nil, err
	}
	defer d.Close()
	rows, err := d.db.Query("SELECT authority_root,aggregate_id,source_root,kind,version,body FROM replica_history ORDER BY authority_root,aggregate_id,kind")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []ReplicatedHistory{}
	for rows.Next() {
		var h ReplicatedHistory
		if err = rows.Scan(&h.AuthorityRoot, &h.AggregateID, &h.SourceRoot, &h.Kind, &h.Version, &h.Fact); err != nil {
			return nil, err
		}
		result = append(result, h)
	}
	return result, rows.Err()
}
