package integration

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
)

type uploadResult struct {
	EntryID   string `json:"entry_id"`
	VersionID string `json:"version_id"`
	Reused    bool   `json:"reused"`
}

type downloadResult struct {
	EntryID       string `json:"entry_id"`
	PullID        string `json:"pull_id"`
	FirstDelivery bool   `json:"first_delivery"`
}

type harness struct {
	t           *testing.T
	server      string
	cli         string
	scope       string
	domain      string
	owner       string
	device      string
	dedupSecret string
}

func TestTransferLifecycle(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	h := &harness{
		t: t, server: requiredEnv("PINELINK_TEST_SERVER"), cli: requiredEnv("PINELINK_TEST_CLI"),
		scope: "integration-" + uuid.NewString(), domain: "integration-domain", owner: "integration-owner",
		device: "integration-device", dedupSecret: "integration-dedup-secret",
	}
	connection, err := nats.Connect(requiredEnv("PINELINK_NATS_URL"), nats.Timeout(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	js, err := connection.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	subscription, err := js.SubscribeSync("events.pinelink.>", nats.DeliverNew(), nats.ManualAck())
	if err != nil {
		t.Fatal(err)
	}
	defer subscription.Unsubscribe()

	directory := t.TempDir()
	input := filepath.Join(directory, "input.bin")
	data := make([]byte, 200_000)
	for index := range data {
		data[index] = byte(index * 31)
	}
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}

	first := h.upload(input, "PERSISTENT")
	second := h.upload(input, "PERSISTENT")
	if first.Reused || !second.Reused {
		t.Fatalf("unexpected reuse results: first=%+v second=%+v", first, second)
	}
	if first.VersionID != second.VersionID || first.EntryID == second.EntryID {
		t.Fatalf("physical and logical identities are wrong: %+v %+v", first, second)
	}

	outputOne := filepath.Join(directory, "one.bin")
	outputTwo := filepath.Join(directory, "two.bin")
	downloadOne := h.download(second.EntryID, outputOne, false)
	downloadTwo := h.download(second.EntryID, outputTwo, false)
	if !downloadOne.FirstDelivery || downloadTwo.FirstDelivery {
		t.Fatalf("first delivery semantics are wrong: %+v %+v", downloadOne, downloadTwo)
	}
	if downloadOne.PullID == downloadTwo.PullID {
		t.Fatal("repeated downloads reused a pull session")
	}
	assertFileEquals(input, outputOne, t)
	assertFileEquals(input, outputTwo, t)

	outputResume := filepath.Join(directory, "resume.bin")
	h.writeDownloadPrefix(second.EntryID, outputResume+".part", 1000)
	h.download(second.EntryID, outputResume, true)
	assertFileEquals(input, outputResume, t)

	pool, err := pgxpool.New(ctx, requiredEnv("PINELINK_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var versions, entries, pulls, receipts int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM file_versions WHERE scope_id=$1`, h.scope).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM file_entries WHERE version_id=$1`, first.VersionID).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM pull_sessions WHERE entry_id=$1 AND device_id=$2`, second.EntryID, h.device).Scan(&pulls); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM delivery_receipts WHERE entry_id=$1 AND device_id=$2`, second.EntryID, h.device).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if versions != 1 || entries < 2 || pulls != 4 || receipts != 1 {
		t.Fatalf("unexpected database state: versions=%d entries=%d pulls=%d receipts=%d", versions, entries, pulls, receipts)
	}

	temporary := h.upload(input, "TEMP_24H")
	if !temporary.Reused || temporary.VersionID != first.VersionID {
		t.Fatalf("temporary upload did not reuse the sealed version: %+v", temporary)
	}
	if _, err := pool.Exec(ctx, `UPDATE file_entries SET expires_at=now()-interval '1 minute' WHERE id=$1`, temporary.EntryID); err != nil {
		t.Fatal(err)
	}
	if _, err := h.run(temporary.EntryID, filepath.Join(directory, "expired.bin"), false, "download"); err == nil {
		t.Fatal("expired entry unexpectedly allowed a pull")
	}

	otherScope := *h
	otherScope.scope = h.scope + "-other"
	other := otherScope.upload(input, "PERSISTENT")
	if other.VersionID == first.VersionID || other.Reused {
		t.Fatalf("cross-scope dedup leaked content existence: %+v", other)
	}

	resumeScope := *h
	resumeScope.scope = h.scope + "-resume"
	resumeUploadID := resumeScope.partialUpload(input)
	resumed := resumeScope.upload(input, "PERSISTENT")
	if resumed.Reused {
		t.Fatalf("resumed upload unexpectedly became a full-file reuse: %+v", resumed)
	}
	var completedEntryID string
	if err := pool.QueryRow(ctx, `SELECT id FROM file_entries WHERE upload_id=$1`, resumeUploadID).Scan(&completedEntryID); err != nil {
		t.Fatal(err)
	}
	if completedEntryID != resumed.EntryID {
		t.Fatalf("resume created a different upload session: %s != %s", completedEntryID, resumed.EntryID)
	}
	h.completeUploadAgain(resumeUploadID, resumed.EntryID)

	assertEvents(ctx, t, subscription, map[string]bool{
		"SPOOL_SEALED": false, "PERSISTENCE_CHANGED": false, "UPLOAD_REUSED": false,
		"PULL_COMPLETED": false, "DELIVERY_CONFIRMED": false,
	})
}

func (h *harness) partialUpload(path string) string {
	output, err := h.command("upload", path, "--retention", "PERSISTENT", "--chunk-size", "65536", "--max-chunks", "1")
	if err == nil || !strings.Contains(output, "partial upload stopped") {
		h.t.Fatalf("partial upload did not stop as requested: err=%v output=%s", err, output)
	}
	for _, field := range strings.Fields(output) {
		if strings.HasPrefix(field, "upload_id=") {
			return strings.TrimPrefix(field, "upload_id=")
		}
	}
	h.t.Fatalf("partial upload ID missing: %s", output)
	return ""
}

func (h *harness) interruptedUpload(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		h.t.Fatal(err)
	}
	root := sha256.Sum256(data)
	mac := hmac.New(sha256.New, []byte(h.dedupSecret))
	_, _ = fmt.Fprintf(mac, "sha256-v1\x00%d\x00%s", len(data), hex.EncodeToString(root[:]))
	request := map[string]any{
		"scope_id": h.scope, "encryption_domain_id": h.domain,
		"owner_id": h.owner, "device_id": h.device, "file_name": "resume.bin",
		"retention_class": "PERSISTENT", "hash_version": "sha256-v1",
		"dedup_key": hex.EncodeToString(mac.Sum(nil)), "content_root": hex.EncodeToString(root[:]),
		"size_bytes": len(data), "chunk_size": 65536,
	}
	var prepared struct {
		UploadID string `json:"upload_id"`
	}
	h.apiJSON(http.MethodPost, "/v1/uploads/prepare", request, &prepared)
	chunk := data[:65536]
	digest := sha256.Sum256(chunk)
	for attempt := 0; attempt < 2; attempt++ {
		req, err := http.NewRequest(http.MethodPut, h.server+"/v1/uploads/"+prepared.UploadID+"/chunks/0", bytes.NewReader(chunk))
		if err != nil {
			h.t.Fatal(err)
		}
		req.Header.Set("X-Chunk-SHA256", hex.EncodeToString(digest[:]))
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			h.t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			h.t.Fatalf("duplicate chunk attempt %d returned %s", attempt, response.Status)
		}
	}
	return prepared.UploadID
}

func (h *harness) completeUploadAgain(uploadID, expectedEntryID string) {
	var response struct {
		File struct {
			ID string `json:"id"`
		} `json:"file"`
	}
	h.apiJSON(http.MethodPost, "/v1/uploads/"+uploadID+"/complete", nil, &response)
	if response.File.ID != expectedEntryID {
		h.t.Fatalf("idempotent completion returned %s, expected %s", response.File.ID, expectedEntryID)
	}
}

func (h *harness) apiJSON(method, path string, input, output any) {
	var body *bytes.Reader
	if input != nil {
		encoded, err := json.Marshal(input)
		if err != nil {
			h.t.Fatal(err)
		}
		body = bytes.NewReader(encoded)
	} else {
		body = bytes.NewReader(nil)
	}
	req, err := http.NewRequest(method, h.server+path, body)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		h.t.Fatalf("%s %s returned %s", method, path, response.Status)
	}
	if err := json.NewDecoder(response.Body).Decode(output); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) upload(path, retention string) uploadResult {
	output, err := h.run(path, retention, false, "upload")
	if err != nil {
		h.t.Fatal(err)
	}
	var result uploadResult
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		h.t.Fatalf("decode upload output %q: %v", output, err)
	}
	return result
}

func (h *harness) writeDownloadPrefix(entryID, path string, length int64) {
	var pull struct {
		ID string `json:"id"`
	}
	h.apiJSON(http.MethodPost, "/v1/files/"+entryID+"/pulls", map[string]string{"owner_id": h.owner, "device_id": h.device}, &pull)
	req, err := http.NewRequest(http.MethodGet, h.server+"/v1/pulls/"+pull.ID+"/content", nil)
	if err != nil {
		h.t.Fatal(err)
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", length-1))
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusPartialContent {
		h.t.Fatalf("ciphertext prefix returned %s", response.Status)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		h.t.Fatal(err)
	}
	if _, err := io.CopyN(file, response.Body, length); err != nil {
		_ = file.Close()
		h.t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		h.t.Fatal(err)
	}
}

func (h *harness) download(entryID, output string, resume bool) downloadResult {
	text, err := h.run(entryID, output, resume, "download")
	if err != nil {
		h.t.Fatalf("download entry=%s output=%s resume=%t: %v", entryID, output, resume, err)
	}
	var result downloadResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		h.t.Fatal(err)
	}
	return result
}

func (h *harness) run(first, second string, flag bool, command string) (string, error) {
	args := []string{
		"--server", h.server, "--json", "--scope", h.scope,
		"--encryption-domain", h.domain, "--owner", h.owner, "--device", h.device,
		"--dedup-secret", h.dedupSecret, command, first,
	}
	switch command {
	case "upload":
		args = append(args, "--retention", second, "--chunk-size", "65536")
	case "download":
		args = append(args, second)
		if flag {
			args = append(args, "--resume")
		}
	default:
		return "", fmt.Errorf("unknown command %s", command)
	}
	process := exec.Command(h.cli, args...)
	output, err := process.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return string(output), nil
}

func assertEvents(ctx context.Context, t *testing.T, subscription *nats.Subscription, required map[string]bool) {
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		complete := true
		for _, seen := range required {
			complete = complete && seen
		}
		if complete {
			return
		}
		message, err := subscription.NextMsg(time.Second)
		if err != nil {
			continue
		}
		var event struct {
			EventType string `json:"event_type"`
		}
		if err := json.Unmarshal(message.Data, &event); err == nil {
			if _, ok := required[event.EventType]; ok {
				required[event.EventType] = true
			}
		}
		_ = message.Ack()
	}
	t.Fatalf("events not all observed: %+v", required)
}

func assertFileEquals(expected, actual string, t *testing.T) {
	want, err := os.ReadFile(expected)
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(actual)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(want, got) {
		t.Fatalf("%s differs from %s", actual, expected)
	}
}

func requiredEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic("required environment variable is empty: " + key)
	}
	return value
}

func TestJetStreamPublishDeduplication(t *testing.T) {
	connection, err := nats.Connect(requiredEnv("PINELINK_NATS_URL"), nats.Timeout(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	js, err := connection.JetStream()
	if err != nil {
		t.Fatal(err)
	}
	messageID := uuid.NewString()
	for attempt := 0; attempt < 2; attempt++ {
		ack, err := js.Publish("events.pinelink.integration_test", []byte(messageID), nats.MsgId(messageID))
		if err != nil {
			t.Fatal(err)
		}
		if attempt == 1 && !ack.Duplicate {
			t.Fatal("second publish with the same message ID was not deduplicated")
		}
	}
}
