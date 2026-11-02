package integration

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type storageFile struct {
	ID             string `json:"id"`
	VersionID      string `json:"version_id"`
	RetentionClass string `json:"retention_class"`
	State          string `json:"state"`
	Version        int64  `json:"version"`
}

type storageQuota struct {
	UsedBytes  int64 `json:"used_bytes"`
	QuotaBytes int64 `json:"quota_bytes"`
}

func TestBroadcastPromotionAndGC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	sender := &harness{t: t, server: requiredEnv("PINELINK_TEST_SERVER"), cli: requiredEnv("PINELINK_TEST_CLI"),
		scope: "storage-" + uuid.NewString(), domain: "storage-domain", owner: "storage-sender", device: "storage-sender-device", dedupSecret: "storage-secret"}
	directory := t.TempDir()
	input := filepath.Join(directory, "input.bin")
	data := make([]byte, 1<<20)
	for index := range data {
		data[index] = byte(index * 13)
	}
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	uploaded := sender.upload(input, "TEMP_24H")
	promoted := sender.fileAction(t, "promote", uploaded.EntryID)
	if promoted.RetentionClass != "PERSISTENT" || promoted.State != "AVAILABLE" {
		t.Fatalf("promotion failed: %+v", promoted)
	}
	if again := sender.fileAction(t, "promote", uploaded.EntryID); again.Version != promoted.Version {
		t.Fatalf("idempotent promotion changed version: %+v %+v", promoted, again)
	}

	receiverOne := *sender
	receiverOne.owner, receiverOne.device = "storage-receiver", "device-one"
	receiverTwo := receiverOne
	receiverTwo.device = "device-two"
	for _, receiver := range []*harness{&receiverOne, &receiverTwo} {
		if _, err := receiver.command("device", "register"); err != nil {
			t.Fatal(err)
		}
	}
	output, err := sender.command("broadcast", uploaded.EntryID, "--to-owner", receiverOne.owner)
	if err != nil {
		t.Fatal(err)
	}
	var broadcasts lifecycleTransferList
	if err := json.Unmarshal([]byte(output), &broadcasts); err != nil {
		t.Fatal(err)
	}
	if len(broadcasts.Transfers) != 2 {
		t.Fatalf("broadcast did not create per-device requests: %+v", broadcasts)
	}
	for index, receiver := range []*harness{&receiverOne, &receiverTwo} {
		transfer := broadcasts.Transfers[index]
		if transfer.RecipientDeviceID != receiver.device {
			t.Fatalf("broadcast device mismatch: %+v", transfer)
		}
		receiver.transferAction(t, "accept", transfer.ID)
		destination := filepath.Join(directory, receiver.device+".bin")
		receiver.downloadWithTransfer(t, uploaded.EntryID, transfer.ID, destination)
		assertFileEquals(input, destination, t)
	}

	if _, err := receiverTwo.command("device", "revoke"); err != nil {
		t.Fatal(err)
	}
	output, err = sender.command("broadcast", uploaded.EntryID, "--to-owner", receiverOne.owner)
	if err != nil {
		t.Fatal(err)
	}
	var afterRevoke lifecycleTransferList
	if err := json.Unmarshal([]byte(output), &afterRevoke); err != nil {
		t.Fatal(err)
	}
	if len(afterRevoke.Transfers) != 1 || afterRevoke.Transfers[0].RecipientDeviceID != "device-one" {
		t.Fatalf("revoked device received broadcast: %+v", afterRevoke)
	}

	cancelOutput := filepath.Join(directory, "deleted.bin")
	process := receiverOne.start("download", uploaded.EntryID, cancelOutput, "--transfer", broadcasts.Transfers[0].ID, "--rate-limit", "131072")
	receiverOne.waitTask(t, "RUNNING", "")
	sender.fileAction(t, "delete-file", uploaded.EntryID)
	if output, err := watchOutput(process); err == nil || (!strings.Contains(output, "CANCELLED") && !strings.Contains(output, "CONFLICT")) {
		t.Fatalf("file deletion did not interrupt download: err=%v output=%s", err, output)
	}

	pool, err := pgxpool.New(ctx, requiredEnv("PINELINK_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err := pool.Exec(ctx, `UPDATE file_versions SET gc_after=now()-interval '1 second' WHERE id=$1`, uploaded.VersionID); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(15 * time.Second)
	for {
		var state string
		if err := pool.QueryRow(ctx, `SELECT state FROM file_versions WHERE id=$1`, uploaded.VersionID).Scan(&state); err != nil {
			t.Fatal(err)
		}
		if state == "COLLECTED" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("version was not collected, state=%s", state)
		}
		time.Sleep(200 * time.Millisecond)
	}
	var used int64
	if err := pool.QueryRow(ctx, `SELECT used_bytes FROM scope_quotas WHERE scope_id=$1`, sender.scope).Scan(&used); err != nil {
		t.Fatal(err)
	}
	if used != 0 {
		t.Fatalf("GC did not release physical quota: %d", used)
	}

	reuploaded := sender.upload(input, "TEMP_24H")
	if reuploaded.Reused || reuploaded.VersionID == uploaded.VersionID {
		t.Fatalf("collected content was not uploaded as a new physical generation: %+v", reuploaded)
	}
	if err := pool.QueryRow(ctx, `SELECT used_bytes FROM scope_quotas WHERE scope_id=$1`, sender.scope).Scan(&used); err != nil {
		t.Fatal(err)
	}
	var physicalSize int64
	if err := pool.QueryRow(ctx, `SELECT size_bytes FROM file_versions WHERE id=$1`, reuploaded.VersionID).Scan(&physicalSize); err != nil {
		t.Fatal(err)
	}
	if used != physicalSize {
		t.Fatalf("re-upload did not reserve physical quota: used=%d expected=%d", used, physicalSize)
	}

	limited := *sender
	limited.scope = sender.scope + "-limited"
	if _, err := pool.Exec(ctx, `INSERT INTO scope_quotas (scope_id, quota_bytes) VALUES ($1,1)`, limited.scope); err != nil {
		t.Fatal(err)
	}
	if _, err := limited.uploadResult(input); err == nil {
		t.Fatal("upload unexpectedly succeeded beyond physical quota")
	}
	var versions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM file_versions WHERE scope_id=$1`, limited.scope).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 0 {
		t.Fatal("quota rejection left a file version behind")
	}
}

func (h *harness) fileAction(t *testing.T, action, entryID string) storageFile {
	output, err := h.command(action, entryID)
	if err != nil {
		t.Fatalf("%s %s: %v", action, entryID, err)
	}
	var file storageFile
	if err := json.Unmarshal([]byte(output), &file); err != nil {
		t.Fatal(err)
	}
	return file
}

func (h *harness) uploadResult(path string) (string, error) {
	return h.command("upload", path, "--retention", "PERSISTENT", "--chunk-size", "65536")
}
