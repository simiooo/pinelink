package lan

import (
	"encoding/json"
	"fmt"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

type Change struct {
	Sequence    int64           `json:"sequence"`
	EventID     string          `json:"event_id"`
	Kind        string          `json:"kind"`
	AggregateID string          `json:"aggregate_id"`
	Payload     json.RawMessage `json:"payload"`
	CreatedAt   string          `json:"created_at"`
}

type ChangePage struct {
	DeviceID string   `json:"source_device_id"`
	StreamID string   `json:"stream_id"`
	Items    []Change `json:"items"`
	Next     int64    `json:"next_sequence"`
}

func (d *stateDirectory) changes(after int64, limit int, stream string) (ChangePage, error) {
	page := ChangePage{DeviceID: d.deviceID, StreamID: d.streamID, Items: []Change{}, Next: after}
	if after < 0 || limit < 1 || limit > 200 {
		return page, fmt.Errorf("invalid change cursor or page size")
	}
	if stream != "" && stream != d.streamID {
		return page, ErrConflict
	}
	var latest int64
	if err := d.db.QueryRow("SELECT COALESCE(MAX(sequence),0) FROM change_log").Scan(&latest); err != nil {
		return page, err
	}
	if after > latest {
		return page, ErrConflict
	}
	rows, err := d.db.Query("SELECT sequence,event_id,kind,aggregate_id,payload,created_at FROM change_log WHERE sequence>? ORDER BY sequence LIMIT ?", after, limit)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var change Change
		if err = rows.Scan(&change.Sequence, &change.EventID, &change.Kind, &change.AggregateID, &change.Payload, &change.CreatedAt); err != nil {
			return page, err
		}
		page.Items = append(page.Items, change)
		page.Next = change.Sequence
	}
	return page, rows.Err()
}

func Changes(identity *secure.Identity, directory string, after int64, limit int, stream string) (ChangePage, error) {
	root, err := openStateDirectory(directory, identity.LocalID())
	if err != nil {
		return ChangePage{}, err
	}
	defer root.Close()
	return root.changes(after, limit, stream)
}

type StorageInfo struct {
	DeviceID      string `json:"device_id"`
	StreamID      string `json:"stream_id"`
	Schema        int    `json:"schema_version"`
	SQLiteVersion string `json:"sqlite_version"`
	JournalMode   string `json:"journal_mode"`
	Synchronous   int    `json:"synchronous"`
	LastSequence  int64  `json:"last_sequence"`
	UnboundOutbox int64  `json:"unbound_outbox_events"`
	InboxFacts    int64  `json:"inbox_facts"`
	Integrity     string `json:"integrity"`
}

func Storage(identity *secure.Identity, directory string) (StorageInfo, error) {
	root, err := openStateDirectory(directory, identity.LocalID())
	if err != nil {
		return StorageInfo{}, err
	}
	defer root.Close()
	info := StorageInfo{DeviceID: root.deviceID, StreamID: root.streamID}
	queries := []struct {
		sql   string
		value any
	}{
		{"PRAGMA user_version", &info.Schema},
		{"SELECT sqlite_version()", &info.SQLiteVersion},
		{"PRAGMA journal_mode", &info.JournalMode},
		{"PRAGMA synchronous", &info.Synchronous},
		{"SELECT COALESCE(MAX(sequence),0) FROM change_log", &info.LastSequence},
		{"SELECT COUNT(*) FROM sync_outbox WHERE binding_state='UNBOUND'", &info.UnboundOutbox},
		{"SELECT COUNT(*) FROM sync_inbox", &info.InboxFacts},
		{"PRAGMA quick_check", &info.Integrity},
	}
	for _, query := range queries {
		if err = root.db.QueryRow(query.sql).Scan(query.value); err != nil {
			return StorageInfo{}, err
		}
	}
	return info, nil
}

func LocalPeer(identity *secure.Identity, directory string) (Peer, error) {
	root, err := openStateDirectory(directory, identity.LocalID())
	if err != nil {
		return Peer{}, err
	}
	defer root.Close()
	var ready struct {
		Peer           Peer   `json:"peer"`
		ControlAddress string `json:"control_address"`
	}
	if err = readJSON(root, "receiver.json", &ready); err != nil {
		return Peer{}, err
	}
	if ready.Peer.ID != identity.LocalID() {
		return Peer{}, ErrUnauthorized
	}
	if ready.ControlAddress != "" {
		if _, err = endpoint(ready.ControlAddress); err != nil {
			return Peer{}, err
		}
		ready.Peer.Address = ready.ControlAddress
	}
	return ready.Peer, ready.Peer.validate()
}
