package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

type lifecycleTransfer struct {
	ID                string  `json:"id"`
	EntryID           *string `json:"entry_id"`
	DeliveryMode      string  `json:"delivery_mode"`
	State             string  `json:"state"`
	Version           int64   `json:"version"`
	RecipientOwnerID  string  `json:"recipient_owner_id"`
	RecipientDeviceID string  `json:"recipient_device_id"`
}

type lifecycleTransferList struct {
	Transfers []lifecycleTransfer `json:"transfers"`
}

type lifecycleTask struct {
	ID            string `json:"id"`
	ReferenceID   string `json:"reference_id"`
	Kind          string `json:"kind"`
	State         string `json:"state"`
	Version       int64  `json:"version"`
	ProgressBytes int64  `json:"progress_bytes"`
	TotalBytes    int64  `json:"total_bytes"`
}

type lifecycleTaskList struct {
	Tasks []lifecycleTask `json:"tasks"`
}

func TestNotificationsTasksAndConflicts(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	sender := &harness{t: t, server: requiredEnv("PINELINK_TEST_SERVER"), cli: requiredEnv("PINELINK_TEST_CLI"),
		scope: "lifecycle-" + uuid.NewString(), domain: "lifecycle-domain", owner: "sender", device: "sender-device", dedupSecret: "lifecycle-secret"}
	sender.owner = "sender-" + sender.scope
	receiver := *sender
	receiver.owner, receiver.device = "receiver-"+sender.scope, "receiver-device"

	directory := t.TempDir()
	input := filepath.Join(directory, "input.bin")
	data := make([]byte, 2<<20)
	for index := range data {
		data[index] = byte(index * 17)
	}
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	uploaded := sender.upload(input, "PERSISTENT")

	first := sender.send(t, uploaded.EntryID, receiver.owner, receiver.device)
	notifications := receiver.notifications(t)
	if !containsTransfer(notifications.Transfers, first.ID) {
		t.Fatalf("pending notification missing: %+v", notifications)
	}
	if _, err := receiver.command("download", uploaded.EntryID, filepath.Join(directory, "too-early.bin"), "--transfer", first.ID); err == nil {
		t.Fatal("recipient downloaded before accepting")
	}
	receiver.transferAction(t, "reject", first.ID)
	if _, err := receiver.command("accept", first.ID); err == nil {
		t.Fatal("rejected transfer was accepted")
	}

	second := sender.send(t, uploaded.EntryID, receiver.owner, receiver.device)
	receiver.transferAction(t, "accept", second.ID)
	acceptedOutput := filepath.Join(directory, "accepted.bin")
	receiver.downloadWithTransfer(t, uploaded.EntryID, second.ID, acceptedOutput)
	assertFileEquals(input, acceptedOutput, t)

	watch := receiver.start("notifications", "--watch", "--once")
	time.Sleep(300 * time.Millisecond)
	third := sender.send(t, uploaded.EntryID, receiver.owner, receiver.device)
	if output, err := watchOutput(watch); err != nil || !strings.Contains(output, third.ID) {
		t.Fatalf("notification watch failed: %v: %s", err, output)
	}
	// Startup reconciliation also sees a request which arrived before the watch's first poll.
	if output, err := watchOutput(receiver.start("notifications", "--watch", "--once")); err != nil || !strings.Contains(output, third.ID) {
		t.Fatalf("startup notification missing: %v / %s", err, output)
	}
	receiver.transferAction(t, "accept", third.ID)

	staleBody := []byte(fmt.Sprintf(`{"owner_id":%q,"device_id":"sender-device","expected_version":1}`, sender.owner))
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, sender.server+"/v1/transfers/"+third.ID+"/cancel", bytes.NewReader(staleBody))
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var conflict struct {
		Code     string            `json:"code"`
		Resource string            `json:"resource"`
		Current  lifecycleTransfer `json:"current"`
	}
	if err := json.NewDecoder(response.Body).Decode(&conflict); err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusConflict || conflict.Code != "STATE_CONFLICT" || conflict.Resource != "transfer" || conflict.Current.State != "ACCEPTED" {
		t.Fatalf("stale action did not return the current snapshot: status=%d body=%+v", response.StatusCode, conflict)
	}

	pausedOutput := filepath.Join(directory, "paused.bin")
	process := receiver.start("download", uploaded.EntryID, pausedOutput, "--transfer", third.ID, "--rate-limit", "262144")
	task := receiver.waitTask(t, "RUNNING", "")
	receiver.taskAction(t, "pause", task.ID)
	if output, err := watchOutput(process); err == nil || !strings.Contains(output, "PAUSED") {
		t.Fatalf("running download did not observe pause: err=%v output=%s", err, output)
	}
	if info, err := os.Stat(pausedOutput + ".part"); err != nil || info.Size() <= 0 || info.Size() >= int64(len(data)) {
		t.Fatalf("partial download was not preserved: info=%v err=%v", info, err)
	}
	receiver.taskAction(t, "resume", task.ID)
	resumed := receiver.downloadResume(t, uploaded.EntryID, task.ReferenceID, pausedOutput)
	if resumed.PullID != task.ReferenceID {
		t.Fatalf("resume changed pull identity: %+v", resumed)
	}
	assertFileEquals(input, pausedOutput, t)
	receiver.taskAction(t, "delete", task.ID)
	if receiver.taskState(t, task.ID, false) == "DELETED" {
		t.Fatal("deleted task remained in default history")
	}
	if receiver.taskState(t, task.ID, true) != "DELETED" {
		t.Fatal("deleted task missing from audit history")
	}

	fourth := sender.send(t, uploaded.EntryID, receiver.owner, receiver.device)
	receiver.transferAction(t, "accept", fourth.ID)
	cancelOutput := filepath.Join(directory, "cancelled.bin")
	cancelProcess := receiver.start("download", uploaded.EntryID, cancelOutput, "--transfer", fourth.ID, "--rate-limit", "262144")
	cancelTask := receiver.waitTask(t, "RUNNING", task.ID)
	sender.transferAction(t, "cancel", fourth.ID)
	if output, err := watchOutput(cancelProcess); err == nil || !strings.Contains(output, "CANCELLED") {
		t.Fatalf("running download did not observe peer cancellation: err=%v output=%s", err, output)
	}
	if state := receiver.taskState(t, cancelTask.ID, true); state != "CANCELLED" {
		t.Fatalf("peer cancellation left task in %s", state)
	}
	history := receiver.transferHistory(t)
	if !containsTransfer(history.Transfers, first.ID) || !containsTransfer(history.Transfers, fourth.ID) {
		t.Fatal("transfer history is incomplete")
	}
}

func (h *harness) command(args ...string) (string, error) {
	base := []string{"--server", h.server, "--json", "--scope", h.scope,
		"--encryption-domain", h.domain, "--owner", h.owner, "--device", h.device,
		"--dedup-secret", h.dedupSecret}
	process := exec.Command(h.cli, append(base, args...)...)
	output, err := process.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s: %w", strings.TrimSpace(string(output)), err)
	}
	return string(output), nil
}

func (h *harness) start(args ...string) *exec.Cmd {
	base := []string{"--server", h.server, "--json", "--scope", h.scope,
		"--encryption-domain", h.domain, "--owner", h.owner, "--device", h.device,
		"--dedup-secret", h.dedupSecret}
	process := exec.Command(h.cli, append(base, args...)...)
	var output bytes.Buffer
	process.Stdout, process.Stderr = &output, &output
	if err := process.Start(); err != nil {
		h.t.Fatal(err)
	}
	return process
}

func watchOutput(process *exec.Cmd) (string, error) {
	done := make(chan error, 1)
	go func() { done <- process.Wait() }()
	select {
	case err := <-done:
		output := ""
		if process.Stdout != nil {
			if buffer, ok := process.Stdout.(*bytes.Buffer); ok {
				output = buffer.String()
			}
		}
		return output, err
	case <-time.After(45 * time.Second):
		_ = process.Process.Kill()
		return "process timed out", fmt.Errorf("timeout")
	}
}

func (h *harness) send(t *testing.T, entryID, recipientOwner, recipientDevice string) lifecycleTransfer {
	recipient := *h
	recipient.owner, recipient.device = recipientOwner, recipientDevice
	if _, err := recipient.command("device", "register"); err != nil {
		t.Fatal(err)
	}
	output, err := h.command("send", entryID, "--to-owner", recipientOwner, "--to-device", recipientDevice)
	if err != nil {
		t.Fatal(err)
	}
	var transfer lifecycleTransfer
	if err := json.Unmarshal([]byte(output), &transfer); err != nil {
		t.Fatal(err)
	}
	return transfer
}

func (h *harness) notifications(t *testing.T) lifecycleTransferList {
	output, err := h.command("notifications")
	if err != nil {
		t.Fatal(err)
	}
	var list lifecycleTransferList
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func (h *harness) transferAction(t *testing.T, action, id string) lifecycleTransfer {
	output, err := h.command(action, id)
	if err != nil {
		t.Fatal(err)
	}
	var transfer lifecycleTransfer
	if err := json.Unmarshal([]byte(output), &transfer); err != nil {
		t.Fatal(err)
	}
	return transfer
}

func (h *harness) downloadWithTransfer(t *testing.T, entryID, transferID, output string) {
	if _, err := h.command("download", entryID, output, "--transfer", transferID); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) downloadResume(t *testing.T, entryID, pullID, output string) downloadResult {
	text, err := h.command("download", entryID, output, "--pull-id", pullID)
	if err != nil {
		t.Fatal(err)
	}
	var result downloadResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func (h *harness) taskAction(t *testing.T, action, id string) {
	if _, err := h.command("tasks", action, id); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) waitTask(t *testing.T, state, excludeID string) lifecycleTask {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		output, err := h.command("tasks", "list", "--kind", "DOWNLOAD", "--state", state)
		if err != nil {
			t.Fatal(err)
		}
		var list lifecycleTaskList
		if err := json.Unmarshal([]byte(output), &list); err != nil {
			t.Fatal(err)
		}
		for _, task := range list.Tasks {
			if task.ID != excludeID {
				return task
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("no %s task appeared", state)
	return lifecycleTask{}
}

func (h *harness) taskState(t *testing.T, id string, includeDeleted bool) string {
	output, err := h.command("tasks", "list", "--include-deleted="+fmt.Sprint(includeDeleted))
	if err != nil {
		t.Fatal(err)
	}
	var list lifecycleTaskList
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		t.Fatal(err)
	}
	for _, task := range list.Tasks {
		if task.ID == id {
			return task.State
		}
	}
	return ""
}

func (h *harness) transferHistory(t *testing.T) lifecycleTransferList {
	output, err := h.command("transfers", "--role", "recipient")
	if err != nil {
		t.Fatal(err)
	}
	var list lifecycleTransferList
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		t.Fatal(err)
	}
	return list
}

func containsTransfer(transfers []lifecycleTransfer, id string) bool {
	for _, transfer := range transfers {
		if transfer.ID == id {
			return true
		}
	}
	return false
}
