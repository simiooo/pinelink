package lan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	api "github.com/simiooo/pinelink/cli/internal/client"
	"github.com/simiooo/pinelink/cli/internal/secure"
	rp "github.com/simiooo/pinelink/protocol/replication"
)

type SyncTarget struct {
	Target        string  `json:"target"`
	Kind          string  `json:"kind"`
	Address       string  `json:"address"`
	Pin           string  `json:"pin"`
	Owner         string  `json:"owner_id"`
	Device        string  `json:"device_id"`
	Enabled       bool    `json:"enabled"`
	Uploaded      int64   `json:"uploaded_sequence"`
	Epoch         string  `json:"pulled_epoch"`
	Pulled        int64   `json:"pulled_sequence"`
	Bootstrapped  bool    `json:"bootstrapped"`
	SnapshotUpper int64   `json:"snapshot_upper"`
	SnapshotKey   string  `json:"snapshot_key"`
	RosterVersion int64   `json:"roster_version"`
	RosterHash    string  `json:"roster_hash"`
	PendingRoster []byte  `json:"-"`
	Failures      int     `json:"failures"`
	Next          *string `json:"next_attempt_at,omitempty"`
	LastError     string  `json:"last_error,omitempty"`
	LastSuccess   *string `json:"last_success_at,omitempty"`
	BoundEpoch    string  `json:"bound_epoch"`
}

const targetColumns = `target,kind,address,pin,owner_id,device_id,enabled,upload_sequence,pulled_epoch,pulled_sequence,bootstrapped,snapshot_upper,snapshot_key,roster_version,roster_hash,pending_roster,failures,next_attempt_at,last_error,last_success_at,bound_epoch`

func (d *stateDirectory) targets() ([]SyncTarget, error) {
	rows, err := d.db.Query("SELECT " + targetColumns + " FROM replica_targets ORDER BY CASE kind WHEN 'CLOUD' THEN 0 ELSE 1 END,COALESCE(last_success_at,''),target")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []SyncTarget{}
	for rows.Next() {
		var t SyncTarget
		if err = rows.Scan(&t.Target, &t.Kind, &t.Address, &t.Pin, &t.Owner, &t.Device, &t.Enabled, &t.Uploaded, &t.Epoch, &t.Pulled, &t.Bootstrapped, &t.SnapshotUpper, &t.SnapshotKey, &t.RosterVersion, &t.RosterHash, &t.PendingRoster, &t.Failures, &t.Next, &t.LastError, &t.LastSuccess, &t.BoundEpoch); err != nil {
			return nil, err
		}
		result = append(result, t)
	}
	return result, rows.Err()
}
func (d *stateDirectory) ensurePeerTargets() error {
	keys, err := d.keys("peer")
	if err != nil {
		return err
	}
	for _, key := range keys {
		var p Peer
		if err = readJSON(d, key, &p); err != nil {
			return err
		}
		if p.Address == "" {
			continue
		}
		_, err = d.db.Exec(`INSERT INTO replica_targets(target,kind,address,pin,enabled) VALUES(?,'PEER',?,?,?)
ON CONFLICT(target) DO UPDATE SET address=excluded.address,pin=excluded.pin,enabled=excluded.enabled`, "peer:"+p.ID, p.Address, p.SigningKey, !p.Revoked)
		if err != nil {
			return err
		}
	}
	return nil
}
func SyncStatus(identity *secure.Identity, directory string) ([]SyncTarget, error) {
	d, e := openStateDirectory(directory, identity.LocalID())
	if e != nil {
		return nil, e
	}
	defer d.Close()
	if e = d.ensurePeerTargets(); e != nil {
		return nil, e
	}
	return d.targets()
}

func ConfigureCloudSync(identity *secure.Identity, directory, address, pin, owner, device string) error {
	u, e := url.Parse(address)
	if e != nil || u.User != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid history server URL")
	}
	if _, e = rp.Root(pin); e != nil {
		return fmt.Errorf("a pinned history server public key is required")
	}
	if owner == "" || device == "" || len(owner) > 128 || len(device) > 128 {
		return rp.ErrInvalid
	}
	d, e := openStateDirectory(directory, identity.LocalID())
	if e != nil {
		return e
	}
	defer d.Close()
	_, e = d.db.Exec(`INSERT INTO replica_targets(target,kind,address,pin,owner_id,device_id,enabled) VALUES('cloud','CLOUD',?,?,?,?,1)
ON CONFLICT(target) DO UPDATE SET address=excluded.address,pin=excluded.pin,owner_id=excluded.owner_id,device_id=excluded.device_id,enabled=1,bound_epoch='',pulled_epoch='',pulled_sequence=0,bootstrapped=0,snapshot_upper=-1,snapshot_key='',upload_sequence=0,roster_hash='',pending_roster=NULL,failures=0,next_attempt_at=NULL,last_error=''`, strings.TrimRight(address, "/"), pin, owner, device)
	return e
}
func DisableCloudSync(identity *secure.Identity, directory string) error {
	d, e := openStateDirectory(directory, identity.LocalID())
	if e != nil {
		return e
	}
	defer d.Close()
	_, e = d.db.Exec("UPDATE replica_targets SET enabled=0 WHERE target='cloud'")
	return e
}

func remoteInfo(ctx context.Context, address, pin string) (rp.Info, error) {
	nonce := randomToken()
	request, err := http.NewRequestWithContext(ctx, "GET", strings.TrimRight(address, "/")+"/v1/replication/info?nonce="+url.QueryEscape(nonce), nil)
	if err != nil {
		return rp.Info{}, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	c := &http.Client{Transport: transport, Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	defer c.CloseIdleConnections()
	response, err := c.Do(request)
	if err != nil {
		return rp.Info{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return rp.Info{}, fmt.Errorf("history server info returned %d", response.StatusCode)
	}
	var info rp.Info
	if err = json.NewDecoder(io.LimitReader(response.Body, 8<<10)).Decode(&info); err != nil {
		return info, err
	}
	return info, info.Validate(pin, nonce)
}
func (d *stateDirectory) resetRemote(target, epoch string) error {
	_, e := d.db.Exec("UPDATE replica_targets SET pulled_epoch=?,pulled_sequence=0,bootstrapped=0,snapshot_upper=-1,snapshot_key='' WHERE target=?", epoch, target)
	return e
}
func (d *stateDirectory) markUploaded(target, epoch string, sequence int64, cloud bool) error {
	tx, e := d.db.Begin()
	if e != nil {
		return e
	}
	defer tx.Rollback()
	result, e := tx.Exec("UPDATE replica_targets SET upload_sequence=MAX(upload_sequence,?) WHERE target=? AND enabled=1 AND ((kind='CLOUD' AND bound_epoch=?) OR (kind='PEER' AND pulled_epoch=?))", sequence, target, epoch, epoch)
	if e != nil {
		return e
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return ErrConflict
	}
	if cloud {
		if _, e = tx.Exec("UPDATE sync_outbox SET binding_state='CLOUD_ACK' WHERE sequence<=?", sequence); e != nil {
			return e
		}
	}
	return tx.Commit()
}
func (d *stateDirectory) readTarget(name string) (SyncTarget, error) {
	all, e := d.targets()
	if e != nil {
		return SyncTarget{}, e
	}
	for _, t := range all {
		if t.Target == name {
			return t, nil
		}
	}
	return SyncTarget{}, os.ErrNotExist
}

func (d *stateDirectory) roster() ([]rp.Peer, string, error) {
	keys, e := d.keys("peer")
	if e != nil {
		return nil, "", e
	}
	peers := []rp.Peer{}
	for _, key := range keys {
		var p Peer
		if e = readJSON(d, key, &p); e != nil {
			return nil, "", e
		}
		if !p.Revoked {
			peers = append(peers, rp.Peer{Root: p.ID, SigningKey: p.SigningKey, EncryptionKey: p.EncryptionKey})
		}
	}
	slices.SortFunc(peers, func(a, b rp.Peer) int { return strings.Compare(a.Root, b.Root) })
	body, _ := json.Marshal(peers)
	return peers, rp.Hash(body), nil
}
func (d *stateDirectory) syncCloud(ctx context.Context, identity *secure.Identity, t SyncTarget) error {
	info, e := remoteInfo(ctx, t.Address, t.Pin)
	if e != nil {
		return e
	}
	offset := time.Unix(info.UnixTime, 0).Sub(time.Now())
	c, e := api.NewWithClock(t.Address, identity, t.Owner, t.Device, offset)
	if e != nil {
		return e
	}
	if t.BoundEpoch != info.Epoch {
		if _, e = c.DeviceAction(ctx, "register", t.Owner, t.Device, identity.PublicKey(), identity.SigningPublicKey()); e != nil {
			return e
		}
		profile, e := c.BindReplication(ctx, rp.Binding{Root: identity.LocalID(), SigningKey: identity.SigningPublicKey(), EncryptionKey: identity.PublicKey()})
		if e != nil {
			return e
		}
		if e = api.CheckReplicationProfile(profile, t.Pin, identity.LocalID()); e != nil {
			return e
		}
		if _, e = d.db.Exec("UPDATE replica_targets SET bound_epoch=?,upload_sequence=0,roster_hash='',roster_version=?,pending_roster=NULL WHERE target=?", info.Epoch, profile.RosterVersion, t.Target); e != nil {
			return e
		}
		if e = d.resetRemote(t.Target, info.Epoch); e != nil {
			return e
		}
		t, e = d.readTarget(t.Target)
		if e != nil {
			return e
		}
	}
	profile, e := c.ReplicationProfile(ctx, d.streamID)
	if e != nil {
		return e
	}
	if e = api.CheckReplicationProfile(profile, t.Pin, identity.LocalID()); e != nil {
		return e
	}
	peers, hash, e := d.roster()
	if e != nil {
		return e
	}
	if hash != t.RosterHash || len(t.PendingRoster) > 0 {
		var roster rp.Roster
		if len(t.PendingRoster) > 0 {
			if e = json.Unmarshal(t.PendingRoster, &roster); e != nil {
				return e
			}
		} else {
			roster = rp.Roster{OperationID: uuid.NewString(), ExpectedVersion: profile.RosterVersion, Peers: peers}
			body, _ := json.Marshal(roster)
			if _, e = d.db.Exec("UPDATE replica_targets SET pending_roster=? WHERE target=?", body, t.Target); e != nil {
				return e
			}
		}
		p, e := c.ReplicationRoster(ctx, roster)
		if e != nil {
			if strings.Contains(e.Error(), "STATE_CONFLICT") {
				_, _ = d.db.Exec("UPDATE replica_targets SET pending_roster=NULL WHERE target=?", t.Target)
			}
			return e
		}
		if e = api.CheckReplicationProfile(p, t.Pin, identity.LocalID()); e != nil {
			return e
		}
		body, _ := json.Marshal(roster.Peers)
		if _, e = d.db.Exec("UPDATE replica_targets SET roster_hash=?,roster_version=?,pending_roster=NULL WHERE target=?", rp.Hash(body), p.RosterVersion, t.Target); e != nil {
			return e
		}
		// If trust changed while the operation was in flight, publish the new
		// roster first on the next pass, before any new history packets.
		_, currentHash, currentErr := d.roster()
		if currentErr != nil {
			return currentErr
		}
		if rp.Hash(body) != hash || currentHash != hash {
			return nil
		}
	}
	after := profile.Uploaded
	for batchIndex := 0; batchIndex < 4; batchIndex++ {
		batch, e := d.uploadBatch(identity, after, "")
		if e != nil {
			return e
		}
		if batch.Through == after {
			break
		}
		ack, e := c.UploadReplication(ctx, batch)
		if e != nil {
			return e
		}
		if !rp.Verify(t.Pin, ack.Proof(), ack.Signature) || ack.Root != identity.LocalID() || ack.Stream != d.streamID || ack.Epoch != info.Epoch || ack.Through < batch.Through {
			return rp.ErrInvalid
		}
		var latest int64
		if e = d.db.QueryRow("SELECT COALESCE(MAX(sequence),0) FROM change_log").Scan(&latest); e != nil {
			return e
		}
		if ack.Through > latest {
			return fmt.Errorf("history source checkpoint requires recovery")
		}
		if e = d.markUploaded(t.Target, info.Epoch, ack.Through, true); e != nil {
			return e
		}
		after = ack.Through
	}
	if t.Epoch != info.Epoch {
		if e = d.resetRemote(t.Target, info.Epoch); e != nil {
			return e
		}
	}
	for pageIndex := 0; pageIndex < 4; pageIndex++ {
		t, e = d.readTarget(t.Target)
		if e != nil {
			return e
		}
		page, e := c.ReadReplication(ctx, t.Epoch, t.Pulled, !t.Bootstrapped, t.SnapshotUpper, t.SnapshotKey)
		if e != nil {
			if strings.Contains(e.Error(), "STATE_CONFLICT") {
				_ = d.resetRemote(t.Target, info.Epoch)
			}
			return e
		}
		if _, e = d.applyReplicaPage(ctx, identity, t.Target, page, t.Pin); e != nil {
			return e
		}
		if !page.More {
			break
		}
	}
	return nil
}

func (d *stateDirectory) resolvePeer(ctx context.Context, identity *secure.Identity, peer Peer) (*Client, error) {
	client, e := NewClient(identity, peer, d.Name())
	if e != nil {
		return nil, e
	}
	probe, cancel := context.WithTimeout(ctx, 2*time.Second)
	_, e = client.Info(probe)
	cancel()
	if e == nil {
		return client, nil
	}
	client.Close()
	search, cancel := context.WithTimeout(ctx, time.Second)
	nearby, discoverErr := Discover(search)
	cancel()
	if discoverErr != nil {
		return nil, e
	}
	for _, candidate := range nearby {
		if candidate.ID != peer.ID {
			continue
		}
		for _, address := range candidate.Addresses {
			copy := peer
			copy.Address = address
			next, err := NewClient(identity, copy, d.Name())
			if err != nil {
				continue
			}
			probe, cancel := context.WithTimeout(ctx, time.Second)
			_, err = next.Info(probe)
			cancel()
			if err != nil {
				next.Close()
				continue
			}
			// Update only the address, and only if the trust decision is unchanged.
			tx, err := d.db.Begin()
			if err != nil {
				next.Close()
				return nil, err
			}
			var body []byte
			err = tx.QueryRow("SELECT body FROM records WHERE record_key=? AND state='TRUSTED'", "peer-"+peer.ID+".json").Scan(&body)
			var current Peer
			if err == nil {
				err = json.Unmarshal(body, &current)
			}
			if err != nil || current.SigningKey != peer.SigningKey || current.EncryptionKey != peer.EncryptionKey {
				tx.Rollback()
				next.Close()
				return nil, ErrForbidden
			}
			current.Address = address
			body, _ = json.Marshal(current)
			_, err = tx.Exec("UPDATE records SET body=? WHERE record_key=? AND state='TRUSTED'", body, "peer-"+peer.ID+".json")
			if err == nil {
				err = tx.Commit()
			} else {
				tx.Rollback()
			}
			if err != nil {
				next.Close()
				return nil, err
			}
			return next, nil
		}
	}
	return nil, e
}
func (d *stateDirectory) syncPeer(ctx context.Context, identity *secure.Identity, t SyncTarget) error {
	root := strings.TrimPrefix(t.Target, "peer:")
	peer, e := ReadContact(d.Name(), root)
	if e != nil {
		return e
	}
	c, e := d.resolvePeer(ctx, identity, peer)
	if e != nil {
		return e
	}
	defer c.Close()
	nonce := randomToken()
	var info rp.Info
	if e = c.request(ctx, "GET", "replication/info?nonce="+url.QueryEscape(nonce), nil, &info); e != nil {
		return e
	}
	if e = info.Validate(peer.SigningKey, nonce); e != nil {
		return e
	}
	if t.Epoch != info.Epoch {
		if e = d.resetRemote(t.Target, info.Epoch); e != nil {
			return e
		}
		if _, e = d.db.Exec("UPDATE replica_targets SET upload_sequence=0 WHERE target=?", t.Target); e != nil {
			return e
		}
		t.Uploaded = 0
	}
	after := t.Uploaded
	for batchIndex := 0; batchIndex < 4; batchIndex++ {
		batch, e := d.uploadBatch(identity, after, root)
		if e != nil {
			return e
		}
		if batch.Through == after {
			break
		}
		body, _ := json.Marshal(batch)
		var ack rp.Ack
		if e = c.request(ctx, "POST", "replication/events", body, &ack); e != nil {
			return e
		}
		if !rp.Verify(peer.SigningKey, ack.Proof(), ack.Signature) || ack.Root != identity.LocalID() || ack.Stream != d.streamID || ack.Epoch != info.Epoch || ack.Through < batch.Through {
			return rp.ErrInvalid
		}
		if e = d.markUploaded(t.Target, info.Epoch, ack.Through, false); e != nil {
			return e
		}
		after = ack.Through
	}
	for pageIndex := 0; pageIndex < 4; pageIndex++ {
		t, e = d.readTarget(t.Target)
		if e != nil {
			return e
		}
		path := "replication/events"
		if !t.Bootstrapped {
			path = "replication/snapshot"
		}
		query := url.Values{"epoch": []string{t.Epoch}, "after": []string{strconv.FormatInt(t.Pulled, 10)}, "upper": []string{strconv.FormatInt(t.SnapshotUpper, 10)}, "key": []string{t.SnapshotKey}}
		var page rp.Page
		if e = c.request(ctx, "GET", path+"?"+query.Encode(), nil, &page); e != nil {
			var status *StatusError
			if errors.As(e, &status) && status.Status == 409 {
				_ = d.resetRemote(t.Target, info.Epoch)
			}
			return e
		}
		if _, e = d.applyReplicaPage(ctx, identity, t.Target, page, peer.SigningKey); e != nil {
			return e
		}
		if !page.More {
			break
		}
	}
	return nil
}

func (d *stateDirectory) syncPass(ctx context.Context, identity *secure.Identity, force bool) error {
	if e := d.ensurePeerTargets(); e != nil {
		return e
	}
	targets, e := d.targets()
	if e != nil {
		return e
	}
	var errs []error
	visited := 0
	for _, t := range targets {
		if !t.Enabled {
			continue
		}
		if !force && t.Next != nil {
			next, _ := time.Parse(time.RFC3339Nano, *t.Next)
			delay := time.Until(next)
			if delay > 0 && delay <= 65*time.Second {
				continue
			}
		}
		if visited >= 8 {
			break
		}
		visited++
		attempt, cancel := context.WithTimeout(ctx, 12*time.Second)
		if t.Kind == "CLOUD" {
			e = d.syncCloud(attempt, identity, t)
		} else {
			e = d.syncPeer(attempt, identity, t)
		}
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		now := time.Now().UTC()
		if e == nil {
			_, e = d.db.Exec("UPDATE replica_targets SET failures=0,next_attempt_at=?,last_error='',last_success_at=? WHERE target=?", now.Add(time.Second).Format(time.RFC3339Nano), now.Format(time.RFC3339Nano), t.Target)
		} else {
			failures := min(t.Failures+1, 6)
			delay := min(time.Minute, time.Second*time.Duration(1<<failures))
			message := e.Error()
			if len(message) > 300 {
				message = message[:300]
			}
			_, writeErr := d.db.Exec("UPDATE replica_targets SET failures=?,next_attempt_at=?,last_error=? WHERE target=?", failures, now.Add(delay).Format(time.RFC3339Nano), message, t.Target)
			errs = append(errs, fmt.Errorf("%s: %w", t.Target, e))
			if writeErr != nil {
				errs = append(errs, writeErr)
			}
		}
	}
	if ctx.Err() == nil {
		recovery, cancel := context.WithTimeout(ctx, 4*time.Second)
		recoverErr := d.reconcileTransfers(recovery, identity)
		cancel()
		if recoverErr != nil && !errors.Is(recoverErr, context.DeadlineExceeded) {
			errs = append(errs, recoverErr)
		}
	}
	return errors.Join(errs...)
}
func SyncOnce(ctx context.Context, identity *secure.Identity, directory string) error {
	d, e := openStateDirectory(directory, identity.LocalID())
	if e != nil {
		return e
	}
	defer d.Close()
	// SQLite transactions make once safe alongside the daemon; packets and
	// cursors are idempotent, while the daemon's lease prevents duplicate loops.
	return d.syncPass(ctx, identity, true)
}
func RunSync(ctx context.Context, identity *secure.Identity, directory string) error {
	d, e := openStateDirectory(directory, identity.LocalID())
	if e != nil {
		return e
	}
	defer d.Close()
	lock, e := d.OpenFile("sync.lock", os.O_CREATE|os.O_RDWR, 0o600)
	if e != nil {
		return e
	}
	defer lock.Close()
	if e = lockDirectory(lock); e != nil {
		return fmt.Errorf("a history sync worker is already running")
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var jobs recoveryJobs
	defer jobs.group.Wait()
	for {
		_ = d.syncPass(ctx, identity, false)
		jobs.launch(ctx, d, identity)
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
