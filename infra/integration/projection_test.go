package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestFailureAndStableProjection(t *testing.T) {
	h := &harness{t: t, server: requiredEnv("PINELINK_TEST_SERVER"), cli: requiredEnv("PINELINK_TEST_CLI"),
		scope: "projection-" + uuid.NewString(), domain: "projection-domain", owner: "projection-owner", device: "projection-device", dedupSecret: "projection-secret"}
	directory := t.TempDir()
	input := filepath.Join(directory, "input.bin")
	data := make([]byte, 200_000)
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	uploadID := h.partialUpload(input)
	output, err := h.command("tasks", "list", "--kind", "UPLOAD", "--state", "RUNNING")
	if err != nil {
		t.Fatal(err)
	}
	var list lifecycleTaskList
	if err := json.Unmarshal([]byte(output), &list); err != nil {
		t.Fatal(err)
	}
	var task lifecycleTask
	for _, candidate := range list.Tasks {
		if candidate.ReferenceID == uploadID {
			task = candidate
		}
	}
	if task.ID == "" {
		t.Fatal("upload task was not created")
	}
	output, err = h.command("tasks", "projection", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "SENDING") || strings.Contains(output, "DIRECT") || strings.Contains(output, "RELAY") {
		t.Fatalf("unexpected running projection: %s", output)
	}
	h.taskAction(t, "fail", task.ID)
	output, err = h.command("tasks", "projection", task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output, "FAILED") || !strings.Contains(output, "TASK_FAILED") {
		t.Fatalf("failure was not projected accurately: %s", output)
	}
}
