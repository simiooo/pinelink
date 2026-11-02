package p2p

import (
	"os"
	"path/filepath"
	"github.com/simiooo/pinelink/cli/internal/client"
	"testing"
)

func TestFinalPartialChunkDuplicateDoesNotAppend(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "part")
	size := int64(10)
	tr := client.Transfer{SizeBytes: &size}
	frame := encodeFrame(0, []byte("0123456789"))
	for i := 0; i < 2; i++ {
		if err := receiveFrame(frame, filename, tr, nil); err != nil {
			t.Fatal(err)
		}
	}
	content, err := os.ReadFile(filename)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "0123456789" {
		t.Fatalf("duplicate appended: %q", content)
	}
}

func TestInvalidChunkCannotExtendCheckpointFile(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "part")
	size := int64(10)
	tr := client.Transfer{SizeBytes: &size}
	if err := receiveFrame(encodeFrame(0, []byte("01234567890")), filename, tr, nil); err == nil {
		t.Fatal("accepted oversized final chunk")
	}
	if info, err := os.Stat(filename); err == nil && info.Size() != 0 {
		t.Fatal("invalid chunk mutated checkpoint")
	}
}
