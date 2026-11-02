package lan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/simiooo/pinelink/cli/internal/secure"
	rp "github.com/simiooo/pinelink/protocol/replication"
)

func replicaPair(t *testing.T) (*testReceiver, *testReceiver, *Client) {
	bIdentity, aIdentity := identities(t)
	a := startReceiver(t, aIdentity, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "A", AutoAccept: true})
	b := startReceiver(t, bIdentity, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "B", AutoAccept: true, AutoPair: true})
	c, err := NewClient(aIdentity, b.peer, a.cfg.Directory)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Close)
	invite, err := b.server.Invite()
	if err != nil {
		t.Fatal(err)
	}
	if pair, err := c.Pair(t.Context(), invite, "A"); err != nil || pair.State != "APPROVED" {
		t.Fatal(err)
	}
	if err = c.Remember(); err != nil {
		t.Fatal(err)
	}
	return a, b, c
}

func TestReplicationEncryptedPeerHistoryAndLostAck(t *testing.T) {
	a, b, source := replicaPair(t)
	data := []byte("private metadata, real encrypted file")
	prepared, err := source.PrepareFile(t.Context(), inputFile(t, data))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = source.Send(t.Context(), prepared.ID, nil); err != nil {
		t.Fatal(err)
	}
	batch, err := source.root.uploadBatch(a.identity, 0, b.peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(batch.Events) == 0 {
		t.Fatal("no business packets")
	}
	encoded, _ := json.Marshal(batch)
	if bytes.Contains(encoded, []byte("Design source.fig")) || bytes.Contains(encoded, []byte("plaintext_root")) {
		t.Fatal("private metadata was transmitted as plaintext")
	}
	for _, packet := range batch.Events {
		if packet.Validate() != nil {
			t.Fatal("invalid publication")
		}
		if _, err = b.identity.OpenHistory(packet.Ciphertext); err != nil {
			t.Fatal("recipient cannot decrypt its history", err)
		}
	}
	// Receive and commit, but intentionally do not save the sender's ACK.
	body, _ := json.Marshal(batch)
	var ack rp.Ack
	if err = source.request(t.Context(), "POST", "replication/events", body, &ack); err != nil {
		t.Fatal(err)
	}
	if err = SyncOnce(t.Context(), a.identity, a.cfg.Directory); err != nil {
		t.Fatal("retry after lost ACK failed", err)
	}
	if err = SyncOnce(t.Context(), b.identity, b.cfg.Directory); err != nil {
		t.Fatal(err)
	}
	var count int
	if err = b.server.root.db.QueryRow("SELECT COUNT(*) FROM replica_inbox WHERE source_root=?", a.peer.ID).Scan(&count); err != nil || count != len(batch.Events) {
		t.Fatal("lost ACK duplicated events", count, err)
	}
	for _, r := range []*testReceiver{a, b} {
		history, err := ReplicaHistory(r.identity, r.cfg.Directory)
		if err != nil || len(history) == 0 {
			t.Fatal("history did not converge", err)
		}
	}
	again, err := source.root.uploadBatch(a.identity, 0, b.peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	first, _ := json.Marshal(batch)
	second, _ := json.Marshal(again)
	if !bytes.Equal(first, second) {
		t.Fatal("retry changed ciphertext or signatures")
	}
}

func TestReplicationConcurrentForegroundAndRecoveryShareOneSend(t *testing.T) {
	a, b, source := replicaPair(t)
	prepared, err := source.PrepareFile(t.Context(), inputFile(t, bytes.Repeat([]byte("same send"), 1<<15)))
	if err != nil {
		t.Fatal(err)
	}
	other, err := NewClient(a.identity, b.peer, a.cfg.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	start := make(chan struct{})
	results := make(chan error, 2)
	for _, sender := range []*Client{source, other} {
		go func(c *Client) {
			<-start
			result, err := c.Send(t.Context(), prepared.ID, nil)
			if err == nil && (result.State != "COMPLETED" || result.Receipt == nil) {
				err = errors.New("concurrent sender did not observe completion")
			}
			results <- err
		}(sender)
	}
	close(start)
	for range 2 {
		if err = <-results; err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(b.cfg.ReceiveDirectory)
	if err != nil || len(files) != 1 {
		t.Fatal("concurrent send duplicated files", err)
	}
}

func TestReplicationInboxAndCursorRollbackTogether(t *testing.T) {
	a, b, source := replicaPair(t)
	if _, err := source.PrepareFile(t.Context(), inputFile(t, []byte("pending"))); err != nil {
		t.Fatal(err)
	}
	if err := a.server.root.ensurePeerTargets(); err != nil {
		t.Fatal(err)
	}
	page, err := b.server.root.replicaPage(b.identity, a.peer.ID, "", 0, true, -1, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Items) == 0 {
		t.Fatal("empty snapshot")
	}
	if _, err = a.server.root.db.Exec(`CREATE TRIGGER reject_cursor BEFORE UPDATE ON replica_targets BEGIN SELECT RAISE(ABORT,'injected cursor failure'); END;`); err != nil {
		t.Fatal(err)
	}
	if _, err = a.server.root.applyReplicaPage(t.Context(), a.identity, "peer:"+b.peer.ID, page, b.peer.SigningKey); err == nil {
		t.Fatal("cursor failure committed")
	}
	var count int
	if err = a.server.root.db.QueryRow("SELECT COUNT(*) FROM replica_inbox").Scan(&count); err != nil || count != 0 {
		t.Fatal("inbox escaped transaction rollback", err)
	}
	if _, err = a.server.root.db.Exec("DROP TRIGGER reject_cursor"); err != nil {
		t.Fatal(err)
	}
	if _, err = a.server.root.applyReplicaPage(t.Context(), a.identity, "peer:"+b.peer.ID, page, b.peer.SigningKey); err != nil {
		t.Fatal(err)
	}
	corrupt := page
	corrupt.Items = append([]rp.Delivery(nil), page.Items...)
	corrupt.Items[0].Event.Ciphertext = bytes.Clone(corrupt.Items[0].Event.Ciphertext)
	corrupt.Items[0].Event.Ciphertext[0] ^= 1
	corrupt.Signature = b.identity.Sign(corrupt.Proof())
	if _, err = a.server.root.applyReplicaPage(t.Context(), a.identity, "peer:"+b.peer.ID, corrupt, b.peer.SigningKey); err == nil {
		t.Fatal("mutated ciphertext accepted")
	}
}

func TestReplicationWorkerAutomaticallyRecoversQueuedOfflineSend(t *testing.T) {
	a, b, source := replicaPair(t)
	b.cancel()
	b.server.Close()
	data := bytes.Repeat([]byte("resume offline request"), 5000)
	prepared, err := source.PrepareFile(t.Context(), inputFile(t, data))
	if err == nil || prepared.ID == "" || prepared.State != "PREPARING" {
		t.Fatal("offline request was not durably queued", prepared.State, err)
	}
	if err = source.AutomaticSend(prepared.ID, true); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- RunSync(ctx, a.identity, a.cfg.Directory) }()
	defer func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("worker did not stop")
		}
	}()
	time.Sleep(200 * time.Millisecond)
	listener, err := net.Listen("tcp", b.peer.Address[len("https://"):])
	if err != nil {
		t.Fatal(err)
	}
	replacement, err := NewServer(b.identity, b.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	if err = replacement.Prepare(listener); err != nil {
		t.Fatal(err)
	}
	rctx, rcancel := context.WithCancel(t.Context())
	defer rcancel()
	go replacement.Serve(rctx, listener)
	deadline := time.Now().Add(25 * time.Second)
	var completed sentRecord
	for time.Now().Before(deadline) {
		if readJSON(source.root, sentName(prepared.ID), &completed) == nil && completed.Status.State == "COMPLETED" {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if completed.Status.State != "COMPLETED" || completed.Status.Receipt == nil {
		status, _ := SyncStatus(a.identity, a.cfg.Directory)
		t.Fatalf("automatic recovery failed: state=%s status=%+v", completed.Status.State, status)
	}
	files, err := os.ReadDir(b.cfg.ReceiveDirectory)
	if err != nil || len(files) != 1 {
		t.Fatal("wrong received files", err)
	}
	got, err := os.ReadFile(filepath.Join(b.cfg.ReceiveDirectory, files[0].Name()))
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("recovered plaintext differs", err)
	}
}

func TestReplicationWrongRecipientAndRevokedPeerCannotApply(t *testing.T) {
	a, b, source := replicaPair(t)
	if _, err := source.PrepareFile(t.Context(), inputFile(t, []byte("revoke"))); err != nil {
		t.Fatal(err)
	}
	page, err := b.server.root.replicaPage(b.identity, a.peer.ID, "", 0, true, -1, "")
	if err != nil {
		t.Fatal(err)
	}
	other, err := secure.LoadIdentity("owner", "other")
	if err != nil {
		t.Fatal(err)
	}
	d, err := openStateDirectory(t.TempDir(), other.LocalID())
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if _, err = d.applyReplicaPage(t.Context(), other, "peer:"+b.peer.ID, page, b.peer.SigningKey); err == nil {
		t.Fatal("different recipient applied the page")
	}
	admin, err := NewClient(a.identity, a.peer, a.cfg.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err = admin.Forget(t.Context(), b.peer.ID); err != nil {
		t.Fatal(err)
	}
	if err = SyncOnce(t.Context(), a.identity, a.cfg.Directory); err != nil && !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
	if _, err = a.server.root.applyReplicaPage(t.Context(), a.identity, "peer:"+b.peer.ID, page, b.peer.SigningKey); err != nil {
		t.Fatal(err)
	}
	var applied int
	if err = a.server.root.db.QueryRow("SELECT COUNT(*) FROM replica_history").Scan(&applied); err != nil || applied != 0 {
		t.Fatal("revoked peer history applied", applied, err)
	}
	// A newly confirmed trust relationship can replay previously ignored data.
	if err = writeJSON(a.server.root, "peer-"+b.peer.ID+".json", b.peer); err != nil {
		t.Fatal(err)
	}
	if _, err = a.server.root.applyReplicaPage(t.Context(), a.identity, "peer:"+b.peer.ID, page, b.peer.SigningKey); err != nil {
		t.Fatal(err)
	}
	if err = a.server.root.db.QueryRow("SELECT COUNT(*) FROM replica_history").Scan(&applied); err != nil || applied != len(page.Items) {
		t.Fatal("reconfirmed peer could not replay ignored history", applied, err)
	}
}

func TestReplicationLateResponsesCannotRegressCheckpoints(t *testing.T) {
	a, b, _ := replicaPair(t)
	d := a.server.root
	if err := d.ensurePeerTargets(); err != nil {
		t.Fatal(err)
	}
	target := "peer:" + b.peer.ID
	epoch := b.server.root.streamID
	if err := d.resetRemote(target, epoch); err != nil {
		t.Fatal(err)
	}
	for _, sequence := range []int64{20, 5} {
		if err := d.markUploaded(target, epoch, sequence, false); err != nil {
			t.Fatal(err)
		}
	}
	if err := d.markUploaded(target, a.server.root.streamID, 50, false); !errors.Is(err, ErrConflict) {
		t.Fatal("upload ACK from a different epoch accepted", err)
	}
	page := rp.Page{Root: a.peer.ID, Epoch: epoch, Through: 20, Upper: 20, Items: []rp.Delivery{}}
	page.Signature = b.identity.Sign(page.Proof())
	if _, err := d.applyReplicaPage(t.Context(), a.identity, target, page, b.peer.SigningKey); err != nil {
		t.Fatal(err)
	}
	page.Through, page.Upper = 5, 5
	page.Snapshot, page.More, page.NextKey = true, true, "late-snapshot-page"
	page.Signature = b.identity.Sign(page.Proof())
	if _, err := d.applyReplicaPage(t.Context(), a.identity, target, page, b.peer.SigningKey); err != nil {
		t.Fatal(err)
	}
	status, err := d.readTarget(target)
	if err != nil || status.Uploaded != 20 || status.Pulled != 20 || !status.Bootstrapped || status.SnapshotKey != "" {
		t.Fatal("late response regressed checkpoints", status, err)
	}
}
