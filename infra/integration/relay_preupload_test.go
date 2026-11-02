package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestRelayPreuploadConsentAndRestore(t *testing.T) {
	for _, size := range []int{180_000, 0} {
		name := "file"
		if size == 0 {
			name = "empty"
		}
		t.Run(name, func(t *testing.T) {
			identity := "preupload-" + uuid.NewString()
			sender := &harness{t: t, server: requiredEnv("PINELINK_TEST_SERVER"), cli: requiredEnv("PINELINK_TEST_CLI"), scope: identity, domain: identity, owner: identity + "-sender", device: "sender", dedupSecret: identity}
			receiver := *sender
			receiver.owner = identity + "-receiver"
			receiver.device = "receiver"
			for _, h := range []*harness{sender, &receiver} {
				if _, err := h.command("device", "register"); err != nil {
					t.Fatal(err)
				}
			}
			directory := t.TempDir()
			input := filepath.Join(directory, "source.bin")
			output := filepath.Join(directory, "received.bin")
			if err := os.WriteFile(input, bytes.Repeat([]byte{71}, size), 0o600); err != nil {
				t.Fatal(err)
			}
			process := sender.start("p2p", "send", input, "--to-owner", receiver.owner, "--to-device", receiver.device, "--path", "relay")
			request := receiver.waitP2PRequest(t)
			text, err := watchOutput(process)
			if err != nil {
				t.Fatalf("sender failed before recipient acceptance: %v: %s", err, text)
			}
			var sent struct {
				ID        string `json:"id"`
				State     string `json:"state"`
				Stored    bool   `json:"stored"`
				Delivered bool   `json:"delivered"`
			}
			if err = json.Unmarshal([]byte(text), &sent); err != nil {
				t.Fatal(err)
			}
			if sent.ID != request.ID || sent.State != "PENDING" || !sent.Stored || sent.Delivered {
				t.Fatalf("sender did not finish independently: %s", text)
			}
			history, err := receiver.command("transfers", "--role", "recipient")
			if err != nil {
				t.Fatal(err)
			}
			var list struct {
				Transfers []struct {
					ID             string `json:"id"`
					Envelope       []byte `json:"encrypted_manifest"`
					SenderEnvelope []byte `json:"sender_encrypted_manifest"`
				}
			}
			if err = json.Unmarshal([]byte(history), &list); err != nil {
				t.Fatal(err)
			}
			for _, tr := range list.Transfers {
				if len(tr.Envelope) != 0 || len(tr.SenderEnvelope) != 0 {
					t.Fatal("unaccepted request exposed envelope")
				}
			}
			pool, err := pgxpool.New(t.Context(), requiredEnv("PINELINK_DATABASE_URL"))
			if err != nil {
				t.Fatal(err)
			}
			defer pool.Close()
			var entry string
			var deadline, expiry string
			if err = pool.QueryRow(t.Context(), `SELECT tr.entry_id,tr.expires_at::text,e.expires_at::text FROM transfer_requests tr JOIN file_entries e ON e.id=tr.entry_id WHERE tr.id=$1`, request.ID).Scan(&entry, &deadline, &expiry); err != nil {
				t.Fatal(err)
			}
			if _, err = receiver.command("download", entry, output, "--transfer", request.ID); err == nil {
				t.Fatal("pending recipient downloaded")
			}
			receiver.transferAction(t, "accept", request.ID)
			// The sender process has already exited. A fresh receiver reads only
			// durable chunks and must decrypt, commit and submit the receipt.
			if text, err = receiver.command("p2p", "receive", request.ID, output, "--path", "relay"); err != nil {
				t.Fatalf("offline-sender receive: %v: %s", err, text)
			}
			assertFileEquals(input, output, t)
			restored := filepath.Join(directory, "restored.bin")
			restoredText, err := receiver.command("download", entry, restored, "--transfer", request.ID)
			if err != nil {
				t.Fatal(err)
			}
			var receipt downloadResult
			if err = json.Unmarshal([]byte(restoredText), &receipt); err != nil {
				t.Fatal(err)
			}
			if receipt.FirstDelivery {
				t.Fatal("cloud restore was counted as a new first delivery")
			}
			assertFileEquals(input, restored, t)
			again := filepath.Join(directory, "restored-again.bin")
			receiver.downloadWithTransfer(t, entry, request.ID, again)
			assertFileEquals(input, again, t)
			var afterDeadline, afterExpiry string
			if err = pool.QueryRow(t.Context(), `SELECT tr.expires_at::text,e.expires_at::text FROM transfer_requests tr JOIN file_entries e ON e.id=tr.entry_id WHERE tr.id=$1`, request.ID).Scan(&afterDeadline, &afterExpiry); err != nil {
				t.Fatal(err)
			}
			if deadline != afterDeadline || expiry != afterExpiry {
				t.Fatal("accept/restore extended retention")
			}
			var grants int
			if err = pool.QueryRow(t.Context(), `SELECT count(*) FROM file_access_grants WHERE transfer_id=$1 AND purpose='RESTORE' AND revoked_at IS NULL`, request.ID).Scan(&grants); err != nil || grants != 1 {
				t.Fatalf("restore grant: %d %v", grants, err)
			}
			keptText, err := receiver.command("promote", entry, "--transfer", request.ID)
			if err != nil {
				t.Fatal(err)
			}
			var kept struct {
				ID   string `json:"id"`
				Kind string `json:"reference_kind"`
			}
			if err = json.Unmarshal([]byte(keptText), &kept); err != nil {
				t.Fatal(err)
			}
			if kept.ID == entry || kept.Kind != "PERSISTENT_KEEP" {
				t.Fatalf("keep was not independent: %s", keptText)
			}
			if _, err = sender.command("delete-file", entry); err != nil {
				t.Fatal(err)
			}
			if _, err = receiver.command("download", entry, filepath.Join(directory, "revoked.bin"), "--transfer", request.ID); err == nil {
				t.Fatal("deleted delivery remained accessible")
			}
			privateCopy := filepath.Join(directory, "private-keep.bin")
			receiver.download(kept.ID, privateCopy, false)
			assertFileEquals(input, privateCopy, t)

		})
	}
}
