package lan

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// The receiver owns business state. Other local processes use SQLite
// transactions rather than sharing an in-memory cache or rewriting files.
type stateDirectory struct {
	*os.Root
	db                 *sql.DB
	deviceID, streamID string
}

func (d *stateDirectory) Close() error {
	databaseErr := d.db.Close()
	rootErr := d.Root.Close()
	return errors.Join(databaseErr, rootErr)
}

func openStateDirectory(path, deviceID string) (_ *stateDirectory, resultErr error) {
	root, err := openDirectory(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		if resultErr != nil {
			root.Close()
		}
	}()
	if deviceID == "" {
		if _, err = root.Stat("state.sqlite"); err != nil {
			return nil, err
		}
	}
	// SQLite opens its own journal files. Refuse pre-existing symlinks in the
	// private profile; transfer filenames can never address these filenames.
	for _, name := range []string{"state.sqlite", "state.sqlite-wal", "state.sqlite-shm"} {
		if info, e := root.Lstat(name); e == nil && (!info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0) {
			return nil, fmt.Errorf("invalid local database file")
		}
	}
	// Create privately before SQLite opens the inode. Do not chmod/open/close
	// that inode through os.File while SQLite holds POSIX advisory locks.
	file, createErr := root.OpenFile("state.sqlite", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if createErr == nil {
		if err = file.Close(); err != nil {
			return nil, err
		}
	} else if !os.IsExist(createErr) {
		return nil, createErr
	}
	physical, err := filepath.EvalSymlinks(root.Name())
	if err != nil {
		return nil, err
	}
	filePath := filepath.ToSlash(filepath.Join(physical, "state.sqlite"))
	if filepath.VolumeName(physical) != "" && !strings.HasPrefix(filePath, "/") {
		filePath = "/" + filePath
	}
	location := &url.URL{Scheme: "file", Path: filePath}
	query := url.Values{"_pragma": []string{"busy_timeout(5000)", "journal_mode(WAL)", "foreign_keys(1)", "synchronous(FULL)"}, "_txlock": []string{"immediate"}}
	location.RawQuery = query.Encode()
	database, err := sql.Open("sqlite", location.String())
	if err != nil {
		return nil, err
	}
	database.SetMaxOpenConns(1)
	d := &stateDirectory{Root: root, db: database}
	defer func() {
		if resultErr != nil {
			database.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var version int
	if err = tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return nil, err
	}
	if version > 2 {
		return nil, fmt.Errorf("local database requires a newer client")
	}
	if version == 0 {
		_, err = tx.ExecContext(ctx, `CREATE TABLE database_meta (id INTEGER PRIMARY KEY CHECK(id=1), device_id TEXT NOT NULL, stream_id TEXT NOT NULL, legacy_imported INTEGER NOT NULL DEFAULT 0);
CREATE TABLE records (record_key TEXT PRIMARY KEY, kind TEXT NOT NULL, aggregate_id TEXT NOT NULL, peer_id TEXT NOT NULL, state TEXT NOT NULL, state_version INTEGER NOT NULL, progress_revision INTEGER NOT NULL, acknowledged_bytes INTEGER NOT NULL, body BLOB NOT NULL, updated_at TEXT NOT NULL);
CREATE INDEX records_kind_peer ON records(kind,peer_id,state);
CREATE TABLE change_log (sequence INTEGER PRIMARY KEY AUTOINCREMENT, event_id TEXT NOT NULL UNIQUE, kind TEXT NOT NULL, aggregate_id TEXT NOT NULL, payload BLOB NOT NULL, created_at TEXT NOT NULL);
CREATE TABLE sync_outbox (event_id TEXT PRIMARY KEY REFERENCES change_log(event_id), sequence INTEGER NOT NULL UNIQUE, binding_state TEXT NOT NULL DEFAULT 'UNBOUND', attempts INTEGER NOT NULL DEFAULT 0, next_attempt_at TEXT);
CREATE TABLE sync_inbox (source_device_id TEXT NOT NULL, stream_id TEXT NOT NULL, event_id TEXT NOT NULL, payload_sha256 TEXT NOT NULL, received_at TEXT NOT NULL, PRIMARY KEY(source_device_id,stream_id,event_id));
CREATE TABLE sync_cursors (destination TEXT NOT NULL, source_device_id TEXT NOT NULL, stream_id TEXT NOT NULL, sequence INTEGER NOT NULL CHECK(sequence>=0), PRIMARY KEY(destination,source_device_id,stream_id));
PRAGMA user_version=1;`)
		if err != nil {
			return nil, err
		}
		if len(deviceID) != 64 {
			return nil, ErrUnauthorized
		}
		if _, err = tx.ExecContext(ctx, "INSERT INTO database_meta(id,device_id,stream_id) VALUES(1,?,?)", deviceID, uuid.NewString()); err != nil {
			return nil, err
		}
		version = 1
	}
	if version == 1 {
		_, err = tx.ExecContext(ctx, `CREATE TABLE replica_publications(event_id TEXT PRIMARY KEY REFERENCES change_log(event_id),sequence INTEGER NOT NULL UNIQUE,body BLOB NOT NULL,payload_hash TEXT NOT NULL);
CREATE TABLE replica_targets(target TEXT PRIMARY KEY,kind TEXT NOT NULL,address TEXT NOT NULL,pin TEXT NOT NULL,owner_id TEXT NOT NULL DEFAULT '',device_id TEXT NOT NULL DEFAULT '',enabled INTEGER NOT NULL DEFAULT 1,upload_sequence INTEGER NOT NULL DEFAULT 0,pulled_epoch TEXT NOT NULL DEFAULT '',pulled_sequence INTEGER NOT NULL DEFAULT 0,bootstrapped INTEGER NOT NULL DEFAULT 0,snapshot_upper INTEGER NOT NULL DEFAULT -1,snapshot_key TEXT NOT NULL DEFAULT '',roster_version INTEGER NOT NULL DEFAULT 0,roster_hash TEXT NOT NULL DEFAULT '',pending_roster BLOB,failures INTEGER NOT NULL DEFAULT 0,next_attempt_at TEXT,last_error TEXT NOT NULL DEFAULT '',last_success_at TEXT);
CREATE TABLE replica_inbox(source_root TEXT NOT NULL,stream_id TEXT NOT NULL,event_id TEXT NOT NULL,source_sequence INTEGER NOT NULL,payload_hash TEXT NOT NULL,status TEXT NOT NULL,body BLOB NOT NULL,PRIMARY KEY(source_root,stream_id,event_id));
CREATE TABLE replica_history(authority_root TEXT NOT NULL,aggregate_id TEXT NOT NULL,source_root TEXT NOT NULL,kind TEXT NOT NULL,version INTEGER NOT NULL,stream_id TEXT NOT NULL,sequence INTEGER NOT NULL,body BLOB NOT NULL,event_id TEXT NOT NULL,PRIMARY KEY(authority_root,aggregate_id,source_root,kind));
CREATE TABLE replica_peer_uploads(source_root TEXT NOT NULL,stream_id TEXT NOT NULL,sequence INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(source_root,stream_id));
ALTER TABLE replica_targets ADD COLUMN bound_epoch TEXT NOT NULL DEFAULT '';
PRAGMA user_version=2;`)
		if err != nil {
			return nil, err
		}
	}
	var imported bool
	if err = tx.QueryRowContext(ctx, "SELECT device_id,stream_id,legacy_imported FROM database_meta WHERE id=1").Scan(&d.deviceID, &d.streamID, &imported); err != nil {
		return nil, err
	}
	if deviceID != "" && deviceID != d.deviceID {
		return nil, ErrUnauthorized
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	if !imported {
		if err = d.importLegacy(); err != nil {
			return nil, err
		}
	}
	return d, nil
}

type recordFields struct {
	kind, id, peer, state  string
	version, revision, ack int64
	payload                []byte
	receipt                *Receipt
	sent                   *sentRecord
}

// Business events exclude secret envelopes and high-frequency byte progress.
type transferFact struct {
	ID          string    `json:"transfer_id"`
	PeerID      string    `json:"peer_id"`
	Direction   string    `json:"direction"`
	State       string    `json:"state"`
	Version     int64     `json:"state_version"`
	FileName    string    `json:"file_name"`
	PlainSize   int64     `json:"plaintext_size"`
	CipherRoot  string    `json:"cipher_root"`
	Authority   string    `json:"authority"`
	AuthorityID string    `json:"authority_device_id"`
	ExpiresAt   time.Time `json:"expires_at"`
	Receipt     *Receipt  `json:"receipt,omitempty"`
	Error       string    `json:"error,omitempty"`
}

func fieldsForRecord(key string, body []byte) (recordFields, error) {
	fields := recordFields{}
	var transfer Transfer
	switch {
	case key == "receiver.json":
		fields.kind, fields.id = "endpoint", "receiver"
		return fields, nil
	case strings.HasPrefix(key, "peer-"):
		var peer Peer
		if err := json.Unmarshal(body, &peer); err != nil {
			return fields, err
		}
		if err := peer.validate(); err != nil {
			return fields, err
		}
		if key != "peer-"+peer.ID+".json" {
			return fields, ErrUnauthorized
		}
		fields.kind, fields.id, fields.peer = "peer", peer.ID, peer.ID
		fields.state = "TRUSTED"
		if peer.Revoked {
			fields.state = "REVOKED"
		}
		fields.payload = body
		return fields, nil
	case strings.HasPrefix(key, "transfer-"):
		if err := json.Unmarshal(body, &transfer); err != nil {
			return fields, err
		}
		if key != recordName(transfer.ID) {
			return fields, ErrUnauthorized
		}
		fields.kind = "transfer"
	case strings.HasPrefix(key, "sent-"):
		var sent sentRecord
		if err := json.Unmarshal(body, &sent); err != nil {
			return fields, err
		}
		transfer = sent.Status
		if key != sentName(transfer.ID) || transfer.ID != sent.Offer.ID {
			return fields, ErrUnauthorized
		}
		fields.kind, fields.sent = "sent", &sent
	default:
		return fields, fmt.Errorf("unsupported local record")
	}
	if !validTransferID(transfer.ID) || transfer.Acknowledged < 0 || transfer.Acknowledged > transfer.Size {
		return fields, ErrUnauthorized
	}
	fields.id, fields.peer, fields.state = transfer.ID, transfer.PeerID, transfer.State
	fields.version, fields.revision, fields.ack = transfer.Version, transfer.Revision, transfer.Acknowledged
	fields.receipt = transfer.Receipt
	fact := transferFact{transfer.ID, transfer.PeerID, transfer.Direction, transfer.State, transfer.Version, transfer.FileName, transfer.PlainSize, transfer.CipherRoot, transfer.Authority, transfer.AuthorityID, transfer.ExpiresAt, transfer.Receipt, transfer.Error}
	var err error
	fields.payload, err = json.Marshal(fact)
	return fields, err
}

func writeJSON(root *stateDirectory, key string, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return root.writeRecord(key, body, false)
}

func (d *stateDirectory) writeRecord(key string, body []byte, importing bool) error {
	fields, err := fieldsForRecord(key, body)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var oldState string
	var oldVersion, oldRevision, oldAck int64
	var oldBody []byte
	err = tx.QueryRowContext(ctx, "SELECT state,state_version,progress_revision,acknowledged_bytes,body FROM records WHERE record_key=?", key).Scan(&oldState, &oldVersion, &oldRevision, &oldAck, &oldBody)
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if (fields.kind == "transfer" || fields.kind == "sent") && exists {
		if fields.version < oldVersion || fields.revision < oldRevision || fields.ack < oldAck || (fields.version == oldVersion && fields.state != oldState) {
			return ErrConflict
		}
	}
	// Fence trust and incoming state in the same transaction. Revocation may
	// commit while a chunk is being flushed; it must then prevent its ACK.
	if fields.kind == "transfer" && !importing && fields.state != "CANCELLED" && fields.state != "REJECTED" && fields.state != "EXPIRED" && fields.state != "FAILED" && !(exists && oldState == "COMPLETED" && fields.state == oldState && fields.version == oldVersion) {
		var peerState string
		if err = tx.QueryRowContext(ctx, "SELECT state FROM records WHERE record_key=?", "peer-"+fields.peer+".json").Scan(&peerState); err != nil || peerState != "TRUSTED" {
			return ErrForbidden
		}
	}
	if fields.kind == "sent" && fields.state == "COMPLETED" {
		if fields.receipt == nil || fields.receipt.Verify(fields.sent.Peer, fields.sent.Offer, d.deviceID) != nil || fields.receipt.PlainRoot != fields.sent.PlainRoot || fields.receipt.PlainSize != fields.sent.PlainSize {
			return ErrUnauthorized
		}
		receiptBody, _ := json.Marshal(fields.receipt)
		factID := "receipt:" + fields.receipt.TransferID
		var previousHash string
		err = tx.QueryRowContext(ctx, "SELECT payload_sha256 FROM sync_inbox WHERE source_device_id=? AND stream_id=? AND event_id=?", fields.receipt.RecipientID, "receipt-v1", factID).Scan(&previousHash)
		if err == nil && previousHash != sha256Bytes(receiptBody) {
			return ErrConflict
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if _, err = tx.ExecContext(ctx, "INSERT OR IGNORE INTO sync_inbox(source_device_id,stream_id,event_id,payload_sha256,received_at) VALUES(?,?,?,?,?)", fields.receipt.RecipientID, "receipt-v1", factID, sha256Bytes(receiptBody), time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
			return err
		}
	}
	if bytes.Equal(body, oldBody) {
		return tx.Commit()
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	_, err = tx.ExecContext(ctx, `INSERT INTO records(record_key,kind,aggregate_id,peer_id,state,state_version,progress_revision,acknowledged_bytes,body,updated_at) VALUES(?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(record_key) DO UPDATE SET kind=excluded.kind,aggregate_id=excluded.aggregate_id,peer_id=excluded.peer_id,state=excluded.state,state_version=excluded.state_version,progress_revision=excluded.progress_revision,acknowledged_bytes=excluded.acknowledged_bytes,body=excluded.body,updated_at=excluded.updated_at`, key, fields.kind, fields.id, fields.peer, fields.state, fields.version, fields.revision, fields.ack, body, now)
	if err != nil {
		return err
	}
	businessChange := fields.kind != "endpoint" && (!exists || oldState != fields.state || oldVersion != fields.version || (fields.kind == "peer" && !bytes.Equal(oldBody, body)))
	if businessChange {
		eventID := uuid.NewString()
		kind := strings.ToUpper(fields.kind) + "_CHANGED"
		result, e := tx.ExecContext(ctx, "INSERT INTO change_log(event_id,kind,aggregate_id,payload,created_at) VALUES(?,?,?,?,?)", eventID, kind, fields.id, fields.payload, now)
		if e != nil {
			return e
		}
		sequence, e := result.LastInsertId()
		if e != nil {
			return e
		}
		if fields.kind != "peer" {
			if _, e = tx.ExecContext(ctx, "INSERT INTO sync_outbox(event_id,sequence) VALUES(?,?)", eventID, sequence); e != nil {
				return e
			}
		}
	}
	return tx.Commit()
}

func readJSON(root *stateDirectory, key string, value any) error {
	var body []byte
	err := root.db.QueryRow("SELECT body FROM records WHERE record_key=?", key).Scan(&body)
	if errors.Is(err, sql.ErrNoRows) {
		return os.ErrNotExist
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(body, value)
}

func (d *stateDirectory) keys(kinds ...string) ([]string, error) {
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(kinds)), ",")
	arguments := make([]any, len(kinds))
	for i, kind := range kinds {
		arguments[i] = kind
	}
	rows, err := d.db.Query("SELECT record_key FROM records WHERE kind IN ("+placeholders+") ORDER BY record_key", arguments...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

// Compatibility for local prototypes: import once, retain original files, and
// never reapply their stale snapshots over later SQLite state.
func (d *stateDirectory) importLegacy() error {
	entries, err := fs.ReadDir(d.Root.FS(), ".")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".json") || !(name == "receiver.json" || strings.HasPrefix(name, "peer-") || strings.HasPrefix(name, "transfer-") || strings.HasPrefix(name, "sent-")) {
			continue
		}
		file, e := d.Root.Open(name)
		if e != nil {
			return e
		}
		info, e := file.Stat()
		if e != nil || !info.Mode().IsRegular() || info.Size() > 2<<20 {
			file.Close()
			return fmt.Errorf("invalid legacy record")
		}
		var raw json.RawMessage
		e = json.NewDecoder(file).Decode(&raw)
		file.Close()
		if e != nil {
			return e
		}
		if name == "receiver.json" {
			var ready struct {
				Peer Peer `json:"peer"`
			}
			if e = json.Unmarshal(raw, &ready); e != nil {
				return e
			}
			raw, _ = json.Marshal(ready) // Drop any expired invitation token.
		}
		if e = d.writeRecord(name, raw, true); e != nil {
			return e
		}
	}
	_, err = d.db.Exec("UPDATE database_meta SET legacy_imported=1 WHERE id=1")
	return err
}
