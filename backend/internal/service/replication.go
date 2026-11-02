package service

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	rp "github.com/simiooo/pinelink/protocol/replication"
)

func (s *Service) SetReplicationKey(key ed25519.PrivateKey) error {
	if len(key) != ed25519.PrivateKeySize {
		return ErrInvalid
	}
	s.replicationKey = slices.Clone(key)
	return nil
}
func (s *Service) replicationPublic() string {
	if len(s.replicationKey) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(s.replicationKey.Public().(ed25519.PublicKey))
}
func (s *Service) ReplicationInfo(ctx context.Context, nonce string) (rp.Info, error) {
	if len(s.replicationKey) == 0 {
		return rp.Info{}, fmt.Errorf("replication is not configured")
	}
	if len(nonce) != 43 {
		return rp.Info{}, ErrInvalid
	}
	var epoch string
	if err := s.db.QueryRow(ctx, "SELECT epoch::text FROM replication_meta WHERE id=1").Scan(&epoch); err != nil {
		return rp.Info{}, err
	}
	info := rp.Info{Version: rp.Version, Epoch: epoch, PublicKey: s.replicationPublic(), UnixTime: s.now().Unix(), Nonce: nonce}
	info.Signature = rp.Sign(s.replicationKey, info.Proof())
	return info, nil
}

// Every replication writer takes this lock before touching replication rows.
// Feed counters therefore follow commit order, not nextval allocation order.
func (s *Service) replicationTx(ctx context.Context) (pgx.Tx, string, error) {
	if len(s.replicationKey) == 0 {
		return nil, "", ErrInvalid
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, "", err
	}
	var epoch string
	if err = tx.QueryRow(ctx, "SELECT epoch::text FROM replication_meta WHERE id=1 FOR UPDATE").Scan(&epoch); err != nil {
		tx.Rollback(ctx)
		return nil, "", err
	}
	return tx, epoch, nil
}
func replicationPrincipal(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, owner, device string) (string, error) {
	var root string
	err := q.QueryRow(ctx, `SELECT r.root_id FROM replication_roots r JOIN devices d ON d.owner_id=r.owner_id AND d.device_id=r.device_id
WHERE r.owner_id=$1 AND r.device_id=$2 AND d.status='ACTIVE' AND d.signing_public_key=r.signing_key AND d.public_key=r.encryption_key`, owner, device).Scan(&root)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrUnauthorized
	}
	return root, err
}
func (s *Service) BindReplication(ctx context.Context, owner, device string, b rp.Binding) (rp.Profile, error) {
	id, err := rp.Root(b.SigningKey)
	if err != nil || id != b.Root || !strings.HasPrefix(b.EncryptionKey, "age1") || len(b.EncryptionKey) > 128 {
		return rp.Profile{}, ErrInvalid
	}
	tx, epoch, err := s.replicationTx(ctx)
	if err != nil {
		return rp.Profile{}, err
	}
	defer tx.Rollback(ctx)
	var sign, encryption string
	if err = tx.QueryRow(ctx, "SELECT COALESCE(signing_public_key,''),COALESCE(public_key,'') FROM devices WHERE owner_id=$1 AND device_id=$2 AND status='ACTIVE'", owner, device).Scan(&sign, &encryption); err != nil || sign != b.SigningKey || encryption != b.EncryptionKey {
		return rp.Profile{}, ErrUnauthorized
	}
	if _, err = tx.Exec(ctx, `INSERT INTO replication_roots(root_id,owner_id,device_id,signing_key,encryption_key) VALUES($1,$2,$3,$4,$5)
ON CONFLICT(root_id) DO NOTHING`, b.Root, owner, device, b.SigningKey, b.EncryptionKey); err != nil {
		return rp.Profile{}, err
	}
	root, err := replicationPrincipal(ctx, tx, owner, device)
	if err != nil || root != b.Root {
		return rp.Profile{}, ErrUnauthorized
	}
	if _, err = tx.Exec(ctx, "INSERT INTO replication_feeds(root_id) VALUES($1) ON CONFLICT DO NOTHING", root); err != nil {
		return rp.Profile{}, err
	}
	var version int64
	if err = tx.QueryRow(ctx, "SELECT roster_version FROM replication_roots WHERE root_id=$1", root).Scan(&version); err != nil {
		return rp.Profile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return rp.Profile{}, err
	}
	profile := rp.Profile{Root: root, Epoch: epoch, RosterVersion: version}
	profile.Signature = rp.Sign(s.replicationKey, profile.Proof())
	return profile, nil
}
func (s *Service) ReplicationProfile(ctx context.Context, owner, device, stream string) (rp.Profile, error) {
	root, err := replicationPrincipal(ctx, s.db, owner, device)
	if err != nil {
		return rp.Profile{}, err
	}
	p := rp.Profile{Root: root}
	if stream != "" && !rp.ValidID(stream) {
		return p, ErrInvalid
	}
	err = s.db.QueryRow(ctx, `SELECT m.epoch::text,r.roster_version,COALESCE((SELECT uploaded_through FROM replication_sources WHERE root_id=r.root_id AND stream_id=NULLIF($2,'')::uuid),0)
FROM replication_meta m CROSS JOIN replication_roots r WHERE m.id=1 AND r.root_id=$1`, root, stream).Scan(&p.Epoch, &p.RosterVersion, &p.Uploaded)
	if err != nil {
		return p, err
	}
	p.Signature = rp.Sign(s.replicationKey, p.Proof())
	return p, nil
}
func (s *Service) UpdateReplicationRoster(ctx context.Context, owner, device string, r rp.Roster) (rp.Profile, error) {
	if !rp.ValidID(r.OperationID) || r.ExpectedVersion < 0 || len(r.Peers) > 1024 {
		return rp.Profile{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, p := range r.Peers {
		if p.Validate() != nil || seen[p.Root] {
			return rp.Profile{}, ErrInvalid
		}
		seen[p.Root] = true
	}
	encoded, _ := json.Marshal(r)
	tx, epoch, err := s.replicationTx(ctx)
	if err != nil {
		return rp.Profile{}, err
	}
	defer tx.Rollback(ctx)
	root, err := replicationPrincipal(ctx, tx, owner, device)
	if err != nil {
		return rp.Profile{}, err
	}
	if seen[root] {
		return rp.Profile{}, ErrInvalid
	}
	var version int64
	var previousHash string
	if err = tx.QueryRow(ctx, "SELECT roster_version FROM replication_roots WHERE root_id=$1", root).Scan(&version); err != nil {
		return rp.Profile{}, err
	}
	err = tx.QueryRow(ctx, "SELECT payload_hash FROM replication_operations WHERE root_id=$1 AND operation_id=$2", root, r.OperationID).Scan(&previousHash)
	if err == nil {
		if previousHash != rp.Hash(encoded) {
			return rp.Profile{}, ErrConflict
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		if version != r.ExpectedVersion {
			return rp.Profile{}, ErrConflict
		}
		rows, e := tx.Query(ctx, "SELECT peer_root FROM replication_peers WHERE root_id=$1", root)
		if e != nil {
			return rp.Profile{}, e
		}
		affected := map[string]bool{}
		for rows.Next() {
			var p string
			if e = rows.Scan(&p); e != nil {
				rows.Close()
				return rp.Profile{}, e
			}
			affected[p] = true
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return rp.Profile{}, e
		}
		if _, e = tx.Exec(ctx, "UPDATE replication_peers SET active=false WHERE root_id=$1", root); e != nil {
			return rp.Profile{}, e
		}
		for _, p := range r.Peers {
			affected[p.Root] = true
			if _, e = tx.Exec(ctx, `INSERT INTO replication_peers(root_id,peer_root,signing_key,encryption_key,active) VALUES($1,$2,$3,$4,true)
ON CONFLICT(root_id,peer_root) DO UPDATE SET signing_key=excluded.signing_key,encryption_key=excluded.encryption_key,active=true`, root, p.Root, p.SigningKey, p.EncryptionKey); e != nil {
				return rp.Profile{}, e
			}
		}
		version++
		if _, e = tx.Exec(ctx, "UPDATE replication_roots SET roster_version=$2 WHERE root_id=$1", root, version); e != nil {
			return rp.Profile{}, e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO replication_operations(root_id,operation_id,payload_hash,resulting_version) VALUES($1,$2,$3,$4)", root, r.OperationID, rp.Hash(encoded), version); e != nil {
			return rp.Profile{}, e
		}
		for peer := range affected {
			if e = refreshReplicationLink(ctx, tx, root, peer); e != nil {
				return rp.Profile{}, e
			}
		}
	} else {
		return rp.Profile{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return rp.Profile{}, err
	}
	p := rp.Profile{Root: root, Epoch: epoch, RosterVersion: version}
	p.Signature = rp.Sign(s.replicationKey, p.Proof())
	return p, nil
}
func refreshReplicationLink(ctx context.Context, tx pgx.Tx, a, b string) error {
	if a > b {
		a, b = b, a
	}
	var active bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM replication_peers p JOIN replication_peers q ON q.root_id=p.peer_root AND q.peer_root=p.root_id
JOIN replication_roots r ON r.root_id=p.peer_root JOIN replication_roots t ON t.root_id=p.root_id
WHERE p.root_id=$1 AND p.peer_root=$2 AND p.active AND q.active AND p.signing_key=r.signing_key AND p.encryption_key=r.encryption_key
AND q.signing_key=t.signing_key AND q.encryption_key=t.encryption_key)`, a, b).Scan(&active)
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO replication_links(left_root,right_root) VALUES($1,$2) ON CONFLICT DO NOTHING", a, b); err != nil {
		return err
	}
	var was bool
	var generation int64
	if err = tx.QueryRow(ctx, "SELECT active,generation FROM replication_links WHERE left_root=$1 AND right_root=$2", a, b).Scan(&was, &generation); err != nil {
		return err
	}
	if was == active {
		return nil
	}
	if active {
		generation++
	}
	if _, err = tx.Exec(ctx, "UPDATE replication_links SET active=$3,generation=$4 WHERE left_root=$1 AND right_root=$2", a, b, active, generation); err != nil {
		return err
	}
	if active {
		_, err = tx.Exec(ctx, `INSERT INTO replication_backfills(left_root,right_root,generation,upper_sequence)
SELECT $1,$2,$3,event_sequence FROM replication_meta WHERE id=1 ON CONFLICT DO NOTHING`, a, b, generation)
	}
	return err
}
func insertReplicationEvent(ctx context.Context, tx pgx.Tx, e rp.Envelope) (bool, error) {
	body, _ := json.Marshal(e)
	hash := rp.Hash(body)
	var old string
	err := tx.QueryRow(ctx, "SELECT payload_hash FROM replication_events WHERE source_root=$1 AND stream_id=$2 AND event_id=$3", e.SourceRoot, e.StreamID, e.EventID).Scan(&old)
	if err == nil {
		if old != hash {
			return false, ErrConflict
		}
		return false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	var stored int64
	if err = tx.QueryRow(ctx, "UPDATE replication_meta SET event_sequence=event_sequence+1 WHERE id=1 RETURNING event_sequence").Scan(&stored); err != nil {
		return false, err
	}
	recipients, _ := json.Marshal(e.Recipients)
	_, err = tx.Exec(ctx, `INSERT INTO replication_events(source_root,stream_id,event_id,source_sequence,stored_sequence,aggregate_id,aggregate_version,authority,kind,peer_root,recipients,body,payload_hash)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, e.SourceRoot, e.StreamID, e.EventID, e.Sequence, stored, e.AggregateID, e.AggregateVersion, e.Authority, e.Kind, e.PeerRoot, recipients, body, hash)
	if err != nil {
		var conflict *pgconn.PgError
		if errors.As(err, &conflict) && conflict.Code == "23505" {
			return false, ErrConflict
		}
		return false, err
	}
	return true, nil
}
func replicationDelivery(ctx context.Context, tx pgx.Tx, root string, e rp.Envelope, left, right string, generation int64) error {
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM replication_deliveries WHERE consumer_root=$1 AND source_root=$2 AND stream_id=$3 AND event_id=$4 AND link_generation=$5)`, root, e.SourceRoot, e.StreamID, e.EventID, generation).Scan(&exists); err != nil || exists {
		return err
	}
	var sequence int64
	err := tx.QueryRow(ctx, "UPDATE replication_feeds SET sequence=sequence+1 WHERE root_id=$1 RETURNING sequence", root).Scan(&sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO replication_deliveries(consumer_root,sequence,source_root,stream_id,event_id,link_left,link_right,link_generation)
VALUES($1,$2,$3,$4,$5,NULLIF($6,''),NULLIF($7,''),$8)`, root, sequence, e.SourceRoot, e.StreamID, e.EventID, left, right, generation)
	return err
}
func routeReplicationEvent(ctx context.Context, tx pgx.Tx, e rp.Envelope) error {
	for _, target := range e.Recipients {
		if target == e.SourceRoot || e.Authority == "SERVER" {
			if err := replicationDelivery(ctx, tx, target, e, "", "", 0); err != nil {
				return err
			}
			continue
		}
		a, b := e.SourceRoot, target
		if a > b {
			a, b = b, a
		}
		var generation int64
		err := tx.QueryRow(ctx, "SELECT generation FROM replication_links WHERE left_root=$1 AND right_root=$2 AND active", a, b).Scan(&generation)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return err
		}
		if err = replicationDelivery(ctx, tx, target, e, a, b, generation); err != nil {
			return err
		}
	}
	return nil
}
func (s *Service) UploadReplication(ctx context.Context, owner, device string, b rp.Upload) (rp.Ack, error) {
	if b.Validate() != nil {
		return rp.Ack{}, ErrInvalid
	}
	tx, epoch, err := s.replicationTx(ctx)
	if err != nil {
		return rp.Ack{}, err
	}
	defer tx.Rollback(ctx)
	root, err := replicationPrincipal(ctx, tx, owner, device)
	if err != nil || root != b.SourceRoot {
		return rp.Ack{}, ErrUnauthorized
	}
	var signing string
	if err = tx.QueryRow(ctx, "SELECT signing_key FROM replication_roots WHERE root_id=$1", root).Scan(&signing); err != nil {
		return rp.Ack{}, err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO replication_sources(root_id,stream_id) VALUES($1,$2) ON CONFLICT DO NOTHING", root, b.StreamID); err != nil {
		return rp.Ack{}, err
	}
	var uploaded int64
	if err = tx.QueryRow(ctx, "SELECT uploaded_through FROM replication_sources WHERE root_id=$1 AND stream_id=$2", root, b.StreamID).Scan(&uploaded); err != nil {
		return rp.Ack{}, err
	}
	if b.After > uploaded {
		return rp.Ack{}, ErrConflict
	}
	for _, e := range b.Events {
		if e.SigningKey != signing {
			return rp.Ack{}, ErrUnauthorized
		}
		if e.Sequence <= uploaded {
			var hash string
			body, _ := json.Marshal(e)
			if err = tx.QueryRow(ctx, "SELECT payload_hash FROM replication_events WHERE source_root=$1 AND stream_id=$2 AND event_id=$3", root, e.StreamID, e.EventID).Scan(&hash); err != nil || hash != rp.Hash(body) {
				return rp.Ack{}, ErrConflict
			}
			continue
		}
		if _, err = insertReplicationEvent(ctx, tx, e); err != nil {
			return rp.Ack{}, err
		}
		if err = routeReplicationEvent(ctx, tx, e); err != nil {
			return rp.Ack{}, err
		}
	}
	uploaded = max(uploaded, b.Through)
	if _, err = tx.Exec(ctx, "UPDATE replication_sources SET uploaded_through=$3 WHERE root_id=$1 AND stream_id=$2", root, b.StreamID, uploaded); err != nil {
		return rp.Ack{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return rp.Ack{}, err
	}
	a := rp.Ack{Root: root, Stream: b.StreamID, Through: uploaded, Epoch: epoch}
	a.Signature = rp.Sign(s.replicationKey, a.Proof())
	return a, nil
}

const visibleReplicationDelivery = `(d.link_left IS NULL OR EXISTS(SELECT 1 FROM replication_links l WHERE l.left_root=d.link_left AND l.right_root=d.link_right AND l.active AND l.generation=d.link_generation))`

func (s *Service) ReadReplication(ctx context.Context, owner, device, expectedEpoch string, after int64, limit int, snapshot bool, upper int64, key string) (rp.Page, error) {
	if after < 0 || limit < 1 || limit > rp.MaxBatch || len(key) > 300 {
		return rp.Page{}, ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return rp.Page{}, err
	}
	defer tx.Rollback(ctx)
	root, err := replicationPrincipal(ctx, tx, owner, device)
	if err != nil {
		return rp.Page{}, err
	}
	p := rp.Page{Root: root, After: after, Through: after, Items: []rp.Delivery{}, Snapshot: snapshot}
	if err = tx.QueryRow(ctx, "SELECT m.epoch::text,f.sequence FROM replication_meta m CROSS JOIN replication_feeds f WHERE m.id=1 AND f.root_id=$1", root).Scan(&p.Epoch, &p.Upper); err != nil {
		return p, err
	}
	if (expectedEpoch != "" && p.Epoch != expectedEpoch) || after > p.Upper {
		return p, ErrConflict
	}
	if snapshot && upper >= 0 {
		if upper > p.Upper {
			return p, ErrConflict
		}
		p.Upper = upper
	}
	var rows pgx.Rows
	if snapshot {
		rows, err = tx.Query(ctx, `WITH latest AS (
SELECT DISTINCT ON (e.source_root,e.stream_id,e.kind,e.aggregate_id) d.sequence,e.body,
e.source_root||'|'||e.stream_id::text||'|'||e.kind||'|'||e.aggregate_id::text AS snapshot_key
FROM replication_deliveries d JOIN replication_events e USING(source_root,stream_id,event_id)
WHERE d.consumer_root=$1 AND d.sequence<=$2 AND `+visibleReplicationDelivery+`
ORDER BY e.source_root,e.stream_id,e.kind,e.aggregate_id,d.sequence DESC)
SELECT sequence,body FROM latest WHERE snapshot_key>$3 ORDER BY snapshot_key LIMIT $4`, root, p.Upper, key, limit+1)
	} else {
		rows, err = tx.Query(ctx, `SELECT d.sequence,e.body FROM replication_deliveries d JOIN replication_events e USING(source_root,stream_id,event_id)
WHERE d.consumer_root=$1 AND d.sequence>$2 AND d.sequence<=$3 AND `+visibleReplicationDelivery+` ORDER BY d.sequence LIMIT $4`, root, after, p.Upper, limit+1)
	}
	if err != nil {
		return p, err
	}
	for rows.Next() {
		var d rp.Delivery
		var body []byte
		if err = rows.Scan(&d.Sequence, &body); err != nil {
			rows.Close()
			return p, err
		}
		if err = json.Unmarshal(body, &d.Event); err != nil {
			rows.Close()
			return p, err
		}
		p.Items = append(p.Items, d)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return p, err
	}
	p.More = len(p.Items) > limit
	if p.More {
		p.Items = p.Items[:limit]
	}
	if snapshot {
		if p.More {
			p.NextKey = rp.SnapshotKey(p.Items[len(p.Items)-1].Event)
		}
		p.Through = p.Upper
	}
	if !snapshot {
		if p.More {
			p.Through = p.Items[len(p.Items)-1].Sequence
		} else {
			p.Through = p.Upper
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return p, err
	}
	p.Signature = rp.Sign(s.replicationKey, p.Proof())
	return p, nil
}
