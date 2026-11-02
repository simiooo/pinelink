package hashfile

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func TestComputeAndDedupKey(t *testing.T) {
	data := make([]byte, 100_000)
	for index := range data {
		data[index] = byte(index * 31)
	}
	path := filepath.Join(t.TempDir(), "input.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Compute(path, 64<<10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Size != int64(len(data)) || len(result.ChunkHashes) != 2 {
		t.Fatalf("unexpected result: %+v", result)
	}
	digest := sha256.Sum256(data)
	if result.ContentRoot != hex.EncodeToString(digest[:]) {
		t.Fatal("whole-file hash mismatch")
	}
	if err := Verify(path, result.ContentRoot); err != nil {
		t.Fatal(err)
	}
	first := DedupKey([]byte("domain"), result.Size, result.ContentRoot)
	second := DedupKey([]byte("domain"), result.Size, result.ContentRoot)
	other := DedupKey([]byte("other"), result.Size, result.ContentRoot)
	if first != second || first == other {
		t.Fatal("dedup key is not deterministic and domain-scoped")
	}
}
