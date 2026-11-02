package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

type testBlobs struct {
	mu                           sync.Mutex
	objects                      map[string][]byte
	putGate, getGate, deleteGate func(string)
	deletes                      map[string]int
	deleteErr                    error
}

func (b *testBlobs) PutChunk(ctx context.Context, key string, data []byte) error {
	if b.putGate != nil {
		b.putGate(key)
	} // Deliberately permit a late store-side commit after cancellation.
	b.mu.Lock()
	defer b.mu.Unlock()
	b.objects[key] = bytes.Clone(data)
	return nil
}
func (b *testBlobs) GetChunk(ctx context.Context, key string, start, end int64) (io.ReadCloser, error) {
	if b.getGate != nil {
		b.getGate(key)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	v, ok := b.objects[key]
	if !ok {
		return nil, ErrNotFound
	}
	if start < 0 || end < start || end >= int64(len(v)) {
		return nil, ErrInvalid
	}
	return io.NopCloser(bytes.NewReader(bytes.Clone(v[start : end+1]))), nil
}
func (b *testBlobs) DeleteChunk(ctx context.Context, key string) error {
	if b.deleteGate != nil {
		b.deleteGate(key)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.deleteErr != nil {
		return b.deleteErr
	}
	b.deletes[key]++
	delete(b.objects, key)
	return nil
}
func (b *testBlobs) count() int { b.mu.Lock(); defer b.mu.Unlock(); return len(b.objects) }

func testStorage(t *testing.T) (*Service, *Maintenance, *testBlobs) {
	t.Helper()
	dsn := os.Getenv("PINELINK_SERVICE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set PINELINK_SERVICE_TEST_DATABASE_URL for PostgreSQL concurrency tests")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "sim847_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err = admin.Exec(ctx, `CREATE SCHEMA `+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	sqlDB := stdlib.OpenDB(*cfg.ConnConfig)
	migrations, err := filepath.Abs("../../../infra/migrations")
	if err != nil {
		t.Fatal(err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, sqlDB, os.DirFS(migrations))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = provider.Up(ctx); err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		sqlDB.Close()
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, err := admin.Exec(cleanup, `DROP SCHEMA `+pgx.Identifier{schema}.Sanitize()+` CASCADE`)
		if err != nil {
			t.Error(err)
		}
		admin.Close()
	})
	blobs := &testBlobs{objects: map[string][]byte{}, deletes: map[string]int{}}
	return New(pool, blobs), NewMaintenance(pool, blobs, time.Second, nil), blobs
}

func digest(data []byte) string { x := sha256.Sum256(data); return hex.EncodeToString(x[:]) }
func mustSQL(t *testing.T, s *Service, q string, args ...any) {
	t.Helper()
	if _, err := s.db.Exec(t.Context(), q, args...); err != nil {
		t.Fatal(err)
	}
}
func newTestTransfer(t *testing.T, s *Service, data []byte, sameOwner bool) Transfer {
	t.Helper()
	recipient := "receiver"
	if sameOwner {
		recipient = "sender"
	}
	for _, r := range []DeviceRequest{{OwnerID: "sender", DeviceID: "sender-device"}, {OwnerID: recipient, DeviceID: "receiver-device"}} {
		if _, err := s.RegisterDevice(t.Context(), r); err != nil {
			t.Fatal(err)
		}
	}
	tr, err := s.CreateP2P(t.Context(), P2PCreateRequest{SenderOwnerID: "sender", SenderDeviceID: "sender-device", RecipientOwnerID: recipient, RecipientDeviceID: "receiver-device",
		FileName: "test.bin", ContentRoot: digest(data), SizeBytes: int64(len(data)), EncryptionSuite: "test-envelope", EncryptedManifest: []byte("recipient envelope"), SenderEncryptedManifest: []byte("sender envelope")})
	if err != nil {
		t.Fatal(err)
	}
	return tr
}
func spoolData(t *testing.T, s *Service, tr Transfer, data []byte) uploadObject {
	t.Helper()
	o, err := s.prepareRelaySpool(t.Context(), tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < o.Count; index++ {
		block := data[index*o.ChunkSize : min(len(data), (index+1)*o.ChunkSize)]
		if err = s.UploadChunk(t.Context(), o.ID, index, digest(block), block); err != nil {
			t.Fatal(err)
		}
	}
	return o
}
func transferAction(t *testing.T, s *Service, tr Transfer, action string) Transfer {
	t.Helper()
	current, err := s.GetTransferAny(t.Context(), tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	owner, device := tr.RecipientOwnerID, tr.RecipientDeviceID
	if action == "cancel" {
		owner, device = tr.SenderOwnerID, tr.SenderDeviceID
	}
	current, err = s.RespondTransfer(t.Context(), tr.ID, action, TransferActionRequest{OwnerID: owner, DeviceID: device, ExpectedVersion: current.Version})
	if err != nil {
		t.Fatal(err)
	}
	return current
}
func sealData(t *testing.T, s *Service, tr Transfer, data []byte) (uploadObject, FileEntry) {
	t.Helper()
	o := spoolData(t, s, tr, data)
	r, err := s.CompleteUpload(t.Context(), o.ID)
	if err != nil {
		t.Fatal(err)
	}
	return o, r.File
}

func TestRelayPreuploadGrantsAndPrivateKeep(t *testing.T) {
	s, m, b := testStorage(t)
	ctx := t.Context()
	data := bytes.Repeat([]byte{42}, 70_000)
	// Same-account devices must still require recipient consent for DELIVERY.
	tr := newTestTransfer(t, s, data, true)
	visible, err := s.GetTransfer(ctx, tr.ID, tr.RecipientOwnerID, tr.RecipientDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if len(visible.EncryptedManifest) != 0 || len(visible.SenderEncryptedManifest) != 0 {
		t.Fatal("pending metadata exposed envelopes")
	}
	if _, err = s.AddSignal(ctx, tr.ID, P2PSignalRequest{OwnerID: tr.RecipientOwnerID, DeviceID: tr.RecipientDeviceID, Role: "SENDER", Channel: "relay-uplink-offer", Payload: "spoof"}); err == nil {
		t.Fatal("spoofed sender role accepted")
	}
	if _, err = s.AddSignal(ctx, tr.ID, P2PSignalRequest{OwnerID: tr.RecipientOwnerID, DeviceID: tr.RecipientDeviceID, Role: "RECEIVER", Channel: "relay-downlink-offer", Payload: "pending"}); err == nil {
		t.Fatal("downlink accepted before consent")
	}
	o, e := sealData(t, s, tr, data)
	for _, actor := range [][2]string{{tr.SenderOwnerID, tr.SenderDeviceID}, {tr.RecipientOwnerID, tr.RecipientDeviceID}} {
		events, err := s.SyncEvents(ctx, 0, 100, actor[0], actor[1])
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, event := range events.Events {
			if event.EventType == "SPOOL_SEALED" {
				found = true
			}
			if bytes.Contains(event.Payload, []byte("encrypted_manifest")) {
				t.Fatal("sync exposed an envelope")
			}
		}
		if !found {
			t.Fatal("spool event missing for participant", actor)
		}
	}
	snapshot, err := s.GetP2PProgress(ctx, tr.ID, tr.SenderOwnerID, tr.SenderDeviceID)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Completion.Stored || snapshot.Completion.Delivered || snapshot.Delivery.State != "WAITING_FOR_ACCEPTANCE" {
		t.Fatalf("wrong completion: %+v", snapshot)
	}
	if _, err = s.GetManifest(ctx, e.ID, tr.RecipientOwnerID, tr.RecipientDeviceID, tr.ID); err == nil {
		t.Fatal("pending recipient read manifest")
	}
	if _, err = s.CreatePull(ctx, e.ID, PullCreateRequest{OwnerID: tr.RecipientOwnerID, DeviceID: tr.RecipientDeviceID, TransferID: tr.ID}); err == nil {
		t.Fatal("pending recipient opened content")
	}
	if _, err = s.readRelayChunk(ctx, o.ID, 0); err == nil {
		t.Fatal("pending recipient read relay chunk")
	}
	kept, err := s.PromoteFile(ctx, e.ID, FileActionRequest{OwnerID: tr.SenderOwnerID, DeviceID: tr.SenderDeviceID, ExpectedVersion: 1})
	if err != nil {
		t.Fatal(err)
	}
	if kept.ID == e.ID || kept.ReferenceKind != "PERSISTENT_KEEP" || kept.ExpiresAt != nil {
		t.Fatalf("keep is not private: %+v", kept)
	}
	original, err := s.GetFile(ctx, e.ID, tr.SenderOwnerID)
	if err != nil || original.ExpiresAt == nil {
		t.Fatal("keep altered delivery retention", err)
	}
	transferAction(t, s, tr, "reject")
	if _, err = s.GetManifest(ctx, e.ID, tr.RecipientOwnerID, tr.RecipientDeviceID, tr.ID); err == nil {
		t.Fatal("rejected recipient retained manifest access")
	}
	if _, err = s.GetManifest(ctx, kept.ID, tr.SenderOwnerID, tr.SenderDeviceID, ""); err != nil {
		t.Fatal("private keep lost access", err)
	}
	if err = m.collectVersion(ctx, o.VersionID); err != nil {
		t.Fatal(err)
	}
	if b.count() != o.Count {
		t.Fatal("GC deleted private keep")
	}
	if _, err = s.DeleteFile(ctx, kept.ID, FileActionRequest{OwnerID: tr.SenderOwnerID, DeviceID: tr.SenderDeviceID, ExpectedVersion: kept.Version}); err != nil {
		t.Fatal(err)
	}
	mustSQL(t, s, `UPDATE file_versions SET gc_after=now()-interval '1 second' WHERE id=$1`, o.VersionID)
	var wg sync.WaitGroup
	failures := make(chan error, 2)
	for range 2 {
		wg.Go(func() { failures <- m.collectVersion(ctx, o.VersionID) })
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	var used int64
	if err = s.db.QueryRow(ctx, `SELECT used_bytes FROM scope_quotas WHERE scope_id=$1`, "relay-owner:"+tr.SenderOwnerID).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used != 0 || b.count() != 0 {
		t.Fatalf("GC quota=%d objects=%d", used, b.count())
	}
	if err = m.collectVersion(ctx, o.VersionID); err != nil {
		t.Fatal(err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, count := range b.deletes {
		if count != 1 {
			t.Fatalf("concurrent GC deleted %s %d times", key, count)
		}
	}
}

func TestRejectedLatePutCannotResurrect(t *testing.T) {
	s, m, b := testStorage(t)
	ctx := t.Context()
	data := []byte("late ciphertext")
	tr := newTestTransfer(t, s, data, false)
	o, err := s.prepareRelaySpool(ctx, tr.ID)
	if err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	b.putGate = func(string) { once.Do(func() { close(entered); <-release }) }
	result := make(chan error, 1)
	go func() { result <- s.UploadChunk(ctx, o.ID, 0, digest(data), data) }()
	<-entered
	transferAction(t, s, tr, "reject")
	if err = m.collectVersion(ctx, o.VersionID); err != nil {
		t.Fatal(err)
	}
	var state string
	if err = s.db.QueryRow(ctx, `SELECT state FROM file_versions WHERE id=$1`, o.VersionID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state == "COLLECTED" {
		t.Fatal("live PUT lease did not fence GC")
	}
	// Simulate the store committing after the operation's server-side timeout.
	mustSQL(t, s, `UPDATE object_io_operations SET expires_at=now()-interval '1 second' WHERE version_id=$1`, o.VersionID)
	if err = m.collectVersion(ctx, o.VersionID); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err = <-result; err == nil {
		t.Fatal("late PUT attached after rejection")
	}
	if b.count() != 1 {
		t.Fatal("test did not produce a late store-side commit")
	}
	mustSQL(t, s, `UPDATE object_io_operations SET next_sweep_at=now() WHERE version_id=$1`, o.VersionID)
	if err = m.sweepOrphans(ctx); err != nil {
		t.Fatal(err)
	}
	if b.count() != 0 {
		t.Fatal("late object escaped orphan sweep")
	}
	var entries int
	if err = s.db.QueryRow(ctx, `SELECT count(*) FROM file_entries WHERE version_id=$1`, o.VersionID).Scan(&entries); err != nil || entries != 0 {
		t.Fatal("late write created entry", err)
	}
}

func TestRejectDuringSealAndRead(t *testing.T) {
	for _, phase := range []string{"seal", "read"} {
		t.Run(phase, func(t *testing.T) {
			s, m, b := testStorage(t)
			ctx := t.Context()
			data := []byte("verifiable ciphertext")
			tr := newTestTransfer(t, s, data, false)
			o := spoolData(t, s, tr, data)
			if phase == "read" {
				if _, err := s.CompleteUpload(ctx, o.ID); err != nil {
					t.Fatal(err)
				}
				transferAction(t, s, tr, "accept")
			}
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			b.getGate = func(string) { once.Do(func() { close(entered); <-release }) }
			result := make(chan error, 1)
			go func() {
				if phase == "seal" {
					_, err := s.CompleteUpload(ctx, o.ID)
					result <- err
				} else {
					_, err := s.readRelayChunk(ctx, o.ID, 0)
					result <- err
				}
			}()
			<-entered
			if phase == "seal" {
				snap, err := s.GetP2PProgress(ctx, tr.ID, tr.SenderOwnerID, tr.SenderDeviceID)
				if err != nil || snap.Spool.State != "VERIFYING" || snap.Completion.Stored {
					t.Fatal("seal did not expose a distinct verification state", err)
				}
			}
			action := "reject"
			if phase == "read" {
				action = "cancel"
			}
			transferAction(t, s, tr, action)
			if err := m.collectVersion(ctx, o.VersionID); err != nil {
				t.Fatal(err)
			}
			if b.count() != 1 {
				t.Fatal("I/O lease did not protect an in-flight read")
			}
			close(release)
			if err := <-result; err == nil {
				t.Fatal("revoked operation succeeded")
			}
			if err := m.collectVersion(ctx, o.VersionID); err != nil {
				t.Fatal(err)
			}
			if b.count() != 0 {
				t.Fatal("released delivery was not collected")
			}
			snap, err := s.GetP2PProgress(ctx, tr.ID, tr.SenderOwnerID, tr.SenderDeviceID)
			if err != nil {
				t.Fatal(err)
			}
			if snap.Completion.Stored || snap.Completion.Delivered {
				t.Fatal("terminal attempt was resurrected")
			}
		})
	}
}

func TestRestoreGrantReadLeaseAndExpiry(t *testing.T) {
	s, m, _ := testStorage(t)
	ctx := t.Context()
	data := []byte("restorable ciphertext")
	tr := newTestTransfer(t, s, data, false)
	o, e := sealData(t, s, tr, data)
	accepted := transferAction(t, s, tr, "accept")
	if err := s.UpdateP2PProgress(ctx, tr.ID, P2PProgressRequest{OwnerID: tr.RecipientOwnerID, DeviceID: tr.RecipientDeviceID, ReceivedBytes: int64(len(data)), Route: "RELAY", RouteEpoch: 1}); err != nil {
		t.Fatal(err)
	}
	completed, err := s.CompleteP2P(ctx, tr.ID, TransferActionRequest{OwnerID: tr.RecipientOwnerID, DeviceID: tr.RecipientDeviceID, ExpectedVersion: accepted.Version})
	if err != nil {
		t.Fatal(err)
	}
	if completed.State != "COMPLETED" {
		t.Fatal("receipt missing")
	}
	// Request deadline does not become an implicit restore deadline; the explicit
	// grant follows the unchanged content expiry instead.
	mustSQL(t, s, `UPDATE transfer_requests SET expires_at=now()-interval '1 second' WHERE id=$1`, tr.ID)
	pull, err := s.CreatePull(ctx, e.ID, PullCreateRequest{OwnerID: tr.RecipientOwnerID, DeviceID: tr.RecipientDeviceID, TransferID: tr.ID})
	if err != nil {
		t.Fatal("restore grant not usable", err)
	}
	_, lease, err := s.OpenPullRead(ctx, pull.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteFile(ctx, e.ID, FileActionRequest{OwnerID: tr.SenderOwnerID, DeviceID: tr.SenderDeviceID, ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	if err = s.CheckPullRead(ctx, pull.ID, lease); err == nil {
		t.Fatal("stream retained access after deletion")
	}
	mustSQL(t, s, `UPDATE file_versions SET gc_after=now()-interval '1 second' WHERE id=$1`, o.VersionID)
	if err = m.collectVersion(ctx, o.VersionID); err != nil {
		t.Fatal(err)
	}
	var state string
	_ = s.db.QueryRow(ctx, `SELECT state FROM file_versions WHERE id=$1`, o.VersionID).Scan(&state)
	if state != "SEALED" {
		t.Fatal("active read lease failed to fence GC")
	}
	s.CloseRead(ctx, lease)
	if err = m.collectVersion(ctx, o.VersionID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CreatePull(ctx, e.ID, PullCreateRequest{OwnerID: tr.RecipientOwnerID, DeviceID: tr.RecipientDeviceID, TransferID: tr.ID}); err == nil {
		t.Fatal("deleted copy was restorable")
	}
}

func TestImmutableReuploadAndGCRetry(t *testing.T) {
	s, m, b := testStorage(t)
	ctx := t.Context()
	data := []byte("ciphertext generation one")
	req := PrepareRequest{ScopeID: "private-scope", EncryptionDomainID: "domain", HashVersion: "sha256-v1", DedupKey: digest([]byte("same plaintext")), ContentRoot: digest(data), OwnerID: "owner", DeviceID: "device", FileName: "test", SizeBytes: int64(len(data)), ChunkSize: 64 << 10}
	first, err := s.PrepareUpload(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UploadChunk(ctx, first.UploadID, 0, digest(data), data); err != nil {
		t.Fatal(err)
	}
	sealed, err := s.CompleteUpload(ctx, first.UploadID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.DeleteFile(ctx, sealed.File.ID, FileActionRequest{OwnerID: "owner", DeviceID: "device", ExpectedVersion: 1}); err != nil {
		t.Fatal(err)
	}
	mustSQL(t, s, `UPDATE file_versions SET gc_after=now()-interval '1 second' WHERE id=$1`, first.Manifest.VersionID)
	b.deleteErr = errors.New("temporary storage outage")
	if err = m.collectVersion(ctx, first.Manifest.VersionID); err == nil {
		t.Fatal("delete failure was ignored")
	}
	if _, err = s.PrepareUpload(ctx, req); err == nil {
		t.Fatal("GC generation reopened during partial deletion")
	}
	b.deleteErr = nil
	mustSQL(t, s, `UPDATE object_gc_jobs SET retry_after=now() WHERE version_id=$1`, first.Manifest.VersionID)
	if err = m.collectVersion(ctx, first.Manifest.VersionID); err != nil {
		t.Fatal(err)
	}
	req.ContentRoot = digest([]byte("ciphertext generation two"))
	nextData := []byte("ciphertext generation two")
	next, err := s.PrepareUpload(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if next.Manifest.VersionID == first.Manifest.VersionID {
		t.Fatal("collected version ID was reused")
	}
	if err = s.UploadChunk(ctx, next.UploadID, 0, digest(nextData), nextData); err != nil {
		t.Fatal(err)
	}
	if _, err = s.CompleteUpload(ctx, next.UploadID); err != nil {
		t.Fatal(err)
	}
	if err = m.collectVersion(ctx, first.Manifest.VersionID); err != nil {
		t.Fatal(err)
	}
	mustSQL(t, s, `UPDATE object_io_operations SET next_sweep_at=now() WHERE version_id=$1`, first.Manifest.VersionID)
	if err = m.sweepOrphans(ctx); err != nil {
		t.Fatal(err)
	}
	if b.count() != 1 {
		t.Fatal("old cleanup deleted new generation")
	}
	var root string
	var used int64
	if err = s.db.QueryRow(ctx, `SELECT content_root FROM file_versions WHERE id=$1`, first.Manifest.VersionID).Scan(&root); err != nil {
		t.Fatal(err)
	}
	if root != digest(data) {
		t.Fatal("old history metadata changed")
	}
	if err = s.db.QueryRow(ctx, `SELECT used_bytes FROM scope_quotas WHERE scope_id=$1`, req.ScopeID).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used != int64(len(nextData)) {
		t.Fatalf("quota decremented twice: %d", used)
	}
}

func TestExpiredPreuploadAndCorruptSeal(t *testing.T) {
	for _, phase := range []string{"deadline", "checksum"} {
		t.Run(phase, func(t *testing.T) {
			s, m, b := testStorage(t)
			ctx := t.Context()
			data := []byte("persisted ciphertext")
			tr := newTestTransfer(t, s, data, false)
			o := spoolData(t, s, tr, data)
			if phase == "deadline" {
				mustSQL(t, s, `UPDATE transfer_requests SET expires_at=now()-interval '1 second' WHERE id=$1`, tr.ID)
				if _, err := s.CompleteUpload(ctx, o.ID); err == nil {
					t.Fatal("expired upload sealed")
				}
				if _, err := s.RespondTransfer(ctx, tr.ID, "accept", TransferActionRequest{OwnerID: tr.RecipientOwnerID, DeviceID: tr.RecipientDeviceID, ExpectedVersion: 1}); !errors.Is(err, ErrExpired) {
					t.Fatal("late consent did not expire request", err)
				}
			} else {
				b.mu.Lock()
				for key := range b.objects {
					b.objects[key] = bytes.Repeat([]byte{0}, len(data))
				}
				b.mu.Unlock()
				if _, err := s.CompleteUpload(ctx, o.ID); !errors.Is(err, ErrChecksum) {
					t.Fatal("corrupt ciphertext was sealed", err)
				}
				snap, err := s.GetP2PProgress(ctx, tr.ID, tr.SenderOwnerID, tr.SenderDeviceID)
				if err != nil || snap.Spool.State != "FAILED" || snap.Completion.Stored {
					t.Fatal("failed seal was presented as stored", err)
				}
			}
			if err := m.collectVersion(ctx, o.VersionID); err != nil {
				t.Fatal(err)
			}
			if b.count() != 0 {
				t.Fatal("terminal spool leaked")
			}
		})
	}
}

func TestRelayAdmissionUsesSenderQuota(t *testing.T) {
	s, _, _ := testStorage(t)
	ctx := t.Context()
	data := []byte("bounded")
	ids := make([]string, 5)
	for i := range ids {
		ids[i] = newTestTransfer(t, s, data, false).ID
	}
	for _, id := range ids[:4] {
		if _, err := s.prepareRelaySpool(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.prepareRelaySpool(ctx, ids[4]); !errors.Is(err, ErrConflict) {
		t.Fatal("sender concurrency admission did not reject a fifth spool", err)
	}
	var used int64
	if err := s.db.QueryRow(ctx, `SELECT used_bytes FROM scope_quotas WHERE scope_id='relay-owner:sender'`).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used != 4*int64(len(data)) {
		t.Fatalf("rejected admission charged quota: %d", used)
	}
}

func TestConcurrentSealsAreIdempotent(t *testing.T) {
	s, _, b := testStorage(t)
	ctx := t.Context()
	data := bytes.Repeat([]byte{6}, 80_000)
	tr := newTestTransfer(t, s, data, false)
	o := spoolData(t, s, tr, data)
	entered, release := make(chan struct{}, 2), make(chan struct{})
	var reads atomic.Int32
	b.getGate = func(string) {
		if reads.Add(1) <= 2 {
			entered <- struct{}{}
			<-release
		}
	}
	type result struct {
		entry string
		err   error
	}
	results := make(chan result, 2)
	for range 2 {
		go func() { r, err := s.CompleteUpload(ctx, o.ID); results <- result{r.File.ID, err} }()
	}
	<-entered
	<-entered
	close(release)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil || first.entry == "" || first.entry != second.entry {
		t.Fatalf("concurrent seals: %+v %+v", first, second)
	}
}

func TestGCPagesResumeOnAnotherWorker(t *testing.T) {
	s, m, b := testStorage(t)
	ctx := t.Context()
	data := []byte("one attached chunk")
	tr := newTestTransfer(t, s, data, false)
	o, _ := sealData(t, s, tr, data)
	transferAction(t, s, tr, "reject")
	// A fixed manifest also includes orphan write intents. More than one page
	// forces the worker to persist progress and release its claim.
	mustSQL(t, s, `INSERT INTO object_io_operations(id,version_id,storage_generation,kind,object_key,state,expires_at)
        SELECT gen_random_uuid(),$1,$2,'WRITE','test-orphan-'||i,'ORPHAN',now() FROM generate_series(1,130) i`, o.VersionID, o.Generation)
	if err := m.collectVersion(ctx, o.VersionID); err != nil {
		t.Fatal(err)
	}
	var state string
	var used int64
	if err := s.db.QueryRow(ctx, `SELECT state FROM file_versions WHERE id=$1`, o.VersionID).Scan(&state); err != nil {
		t.Fatal(err)
	}
	if state != "GC_PENDING" {
		t.Fatalf("first page finalized too early: %s", state)
	}
	if err := s.db.QueryRow(ctx, `SELECT used_bytes FROM scope_quotas WHERE scope_id='relay-owner:sender'`).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used != int64(len(data)) {
		t.Fatal("partial GC released physical quota")
	}
	replacement := NewMaintenance(s.db, b, time.Second, nil)
	if err := replacement.collectVersion(ctx, o.VersionID); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(ctx, `SELECT state FROM file_versions WHERE id=$1`, o.VersionID).Scan(&state); err != nil || state != "COLLECTED" {
		t.Fatal("new worker did not resume", state, err)
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	for key, count := range b.deletes {
		if count != 1 {
			t.Fatalf("checkpoint replayed %s %d times", key, count)
		}
	}
}

func TestDeliveryReceiptsIncludeOwner(t *testing.T) {
	s, _, _ := testStorage(t)
	ctx := t.Context()
	data := []byte("shared library object")
	prepared, err := s.PrepareUpload(ctx, PrepareRequest{ScopeID: "scope", EncryptionDomainID: "domain", HashVersion: "sha256-v1", DedupKey: digest(data), ContentRoot: digest(data), OwnerID: "sender", DeviceID: "source", FileName: "shared", SizeBytes: int64(len(data)), ChunkSize: 64 << 10, RetentionClass: "PERSISTENT"})
	if err != nil {
		t.Fatal(err)
	}
	if err = s.UploadChunk(ctx, prepared.UploadID, 0, digest(data), data); err != nil {
		t.Fatal(err)
	}
	sealed, err := s.CompleteUpload(ctx, prepared.UploadID)
	if err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{"alpha", "beta"} {
		tr, err := s.CreateTransfer(ctx, TransferCreateRequest{EntryID: sealed.File.ID, SenderOwnerID: "sender", SenderDeviceID: "source", RecipientOwnerID: owner, RecipientDeviceID: "phone"})
		if err != nil {
			t.Fatal(err)
		}
		if _, err = s.RespondTransfer(ctx, tr.ID, "accept", TransferActionRequest{OwnerID: owner, DeviceID: "phone", ExpectedVersion: tr.Version}); err != nil {
			t.Fatal(err)
		}
		for attempt := range 2 {
			pull, err := s.CreatePull(ctx, sealed.File.ID, PullCreateRequest{OwnerID: owner, DeviceID: "phone", TransferID: tr.ID})
			if err != nil {
				t.Fatal(err)
			}
			receipt, err := s.CompletePull(ctx, pull.ID, PullCompleteRequest{DeviceID: "phone", ContentRoot: digest(data)})
			if err != nil {
				t.Fatal(err)
			}
			if receipt.FirstDelivery != (attempt == 0) {
				t.Fatalf("owner %s attempt %d first=%v", owner, attempt, receipt.FirstDelivery)
			}
		}
	}
}
