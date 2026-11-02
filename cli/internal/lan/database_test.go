package lan

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestDatabaseProjectionLogAndOutboxAreAtomic(t *testing.T) {
	receiver, sender := identities(t)
	directory := t.TempDir()
	db, err := openStateDirectory(directory, sender.LocalID())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	id := uuid.NewString()
	secret := []byte("sealed secret envelope must not enter incremental events")
	status := Transfer{Offer: Offer{ID: id, Size: 10, Envelope: secret}, PeerID: receiver.LocalID(), State: "PREPARING", Direction: "SEND", Authority: "LOCAL_DEVICE", AuthorityID: receiver.LocalID()}
	sent := sentRecord{Offer: status.Offer, Peer: Describe(receiver, "Receiver", "https://127.0.0.1:19091"), OwnEnvelope: secret, Status: status}
	if _, err = db.db.Exec(`CREATE TRIGGER reject_outbox BEFORE INSERT ON sync_outbox BEGIN SELECT RAISE(ABORT,'injected outbox failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if err = writeJSON(db, sentName(id), sent); err == nil {
		t.Fatal("outbox failure committed")
	}
	var loaded sentRecord
	if err = readJSON(db, sentName(id), &loaded); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("projection escaped rollback", err)
	}
	page, err := db.changes(0, 200, "")
	if err != nil || len(page.Items) != 0 {
		t.Fatal("log escaped rollback", err)
	}
	if _, err = db.db.Exec("DROP TRIGGER reject_outbox"); err != nil {
		t.Fatal(err)
	}
	if err = writeJSON(db, sentName(id), sent); err != nil {
		t.Fatal(err)
	}
	sent.Status.State, sent.Status.Version = "ACCEPTED", 1
	if err = writeJSON(db, sentName(id), sent); err != nil {
		t.Fatal(err)
	}
	sent.Status.Acknowledged, sent.Status.Revision = 5, 1
	if err = writeJSON(db, sentName(id), sent); err != nil {
		t.Fatal(err)
	}
	page, err = db.changes(0, 200, "")
	if err != nil || len(page.Items) != 2 {
		t.Fatal("byte progress polluted the business log", len(page.Items), err)
	}
	for _, event := range page.Items {
		if bytes.Contains(event.Payload, secret) || bytes.Contains(event.Payload, []byte("envelope")) || bytes.Contains(event.Payload, []byte("acknowledged_bytes")) {
			t.Fatal("sensitive or high-frequency state entered the log")
		}
	}
	if err = readJSON(db, sentName(id), &loaded); err != nil || loaded.Status.Acknowledged != 5 {
		t.Fatal("progress checkpoint was not updated", err)
	}
	sent.Status.Acknowledged, sent.Status.Revision = 0, 0
	if err = writeJSON(db, sentName(id), sent); !errors.Is(err, ErrConflict) {
		t.Fatal("stale progress overwrote its checkpoint", err)
	}
	info, err := Storage(sender, directory)
	if err != nil || info.Schema != 2 || info.JournalMode != "wal" || info.Synchronous != 2 || info.UnboundOutbox != 2 || info.Integrity != "ok" {
		t.Fatal("wrong durable storage configuration", info, err)
	}
}

func TestRevocationFencesDatabaseAcknowledgement(t *testing.T) {
	receiver, sender := identities(t)
	r := startReceiver(t, receiver, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver", AutoAccept: true})
	source, _ := pairedClient(t, r, sender)
	prepared, err := source.PrepareFile(t.Context(), inputFile(t, bytes.Repeat([]byte("a"), 100_000)))
	if err != nil {
		t.Fatal(err)
	}
	record, _ := r.server.get(prepared.ID)
	// Simulate revocation committing after the chunk's initial trust check,
	// while the transfer lock still belongs to its pending checkpoint.
	record.mu.Lock()
	defer record.mu.Unlock()
	peer := Describe(sender, "Source", "")
	peer.Revoked = true
	if err = writeJSON(r.server.root, "peer-"+peer.ID+".json", peer); err != nil {
		t.Fatal(err)
	}
	next := record.t
	next.Acknowledged, next.Revision = ChunkSize, 1
	if err = r.server.commit(record, next); !errors.Is(err, ErrForbidden) {
		t.Fatal("revoked peer obtained a later ACK", err)
	}
	var saved Transfer
	if err = readJSON(r.server.root, recordName(prepared.ID), &saved); err != nil || saved.Acknowledged != 0 {
		t.Fatal("revoked progress became durable", err)
	}
}

func TestLocalChangeStreamSurvivesRestartAndRejectsWrongIdentity(t *testing.T) {
	receiver, sender := identities(t)
	directory := t.TempDir()
	db, err := openStateDirectory(directory, sender.LocalID())
	if err != nil {
		t.Fatal(err)
	}
	peer := Describe(receiver, "Receiver", "https://127.0.0.1:19091")
	if err = writeJSON(db, "peer-"+peer.ID+".json", peer); err != nil {
		t.Fatal(err)
	}
	first, err := db.changes(0, 1, "")
	if err != nil || len(first.Items) != 1 {
		t.Fatal(err)
	}
	db.Close()
	db, err = openStateDirectory(directory, sender.LocalID())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	empty, err := db.changes(first.Next, 200, first.StreamID)
	if err != nil || len(empty.Items) != 0 || empty.StreamID != first.StreamID {
		t.Fatal("restart replayed acknowledged events", err)
	}
	if _, err = db.changes(first.Next, 200, uuid.NewString()); !errors.Is(err, ErrConflict) {
		t.Fatal("foreign stream cursor was accepted", err)
	}
	if _, err = openStateDirectory(directory, receiver.LocalID()); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("database opened under a different device", err)
	}
}

func TestLegacyImportDoesNotResurrectRevokedTrustOrKeepInviteTokens(t *testing.T) {
	receiver, sender := identities(t)
	directory := t.TempDir()
	peer := Describe(receiver, "Receiver", "https://127.0.0.1:19091")
	body, _ := json.Marshal(peer)
	if err := os.WriteFile(filepath.Join(directory, "peer-"+peer.ID+".json"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	ready, _ := json.Marshal(map[string]any{"peer": Describe(sender, "Source", "https://127.0.0.1:19092"), "connection_code": "do-not-import-this-secret"})
	if err := os.WriteFile(filepath.Join(directory, "receiver.json"), ready, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := openStateDirectory(directory, sender.LocalID())
	if err != nil {
		t.Fatal(err)
	}
	peer.Revoked = true
	if err = writeJSON(db, "peer-"+peer.ID+".json", peer); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err = db.db.QueryRow("SELECT body FROM records WHERE record_key='receiver.json'").Scan(&stored); err != nil || bytes.Contains(stored, []byte("do-not-import")) {
		t.Fatal("invitation entered persistent data", err)
	}
	db.Close()
	db, err = openStateDirectory(directory, sender.LocalID())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var loaded Peer
	if err = readJSON(db, "peer-"+peer.ID+".json", &loaded); err != nil || !loaded.Revoked {
		t.Fatal("old JSON restored trust", err)
	}
	if info, err := os.Stat(filepath.Join(directory, "peer-"+peer.ID+".json")); err != nil || info.ModTime().After(time.Now()) {
		t.Fatal("legacy source was removed", err)
	}
}
