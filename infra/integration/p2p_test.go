package integration

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestP2PDirectRelayAndFallback(t *testing.T) {
	sender := &harness{t: t, server: requiredEnv("PINELINK_TEST_SERVER"), cli: requiredEnv("PINELINK_TEST_CLI"),
		scope: "p2p-" + uuid.NewString(), domain: "p2p-domain", owner: "p2p-sender", device: "p2p-sender-device", dedupSecret: "p2p-secret"}
	receiver := *sender
	receiver.owner, receiver.device = "p2p-receiver", "p2p-receiver-device"
	directory := t.TempDir()
	input := filepath.Join(directory, "input.bin")
	data := make([]byte, 300_000)
	for index := range data {
		data[index] = byte(index * 29)
	}
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := sender.command("device", "register"); err != nil {
		t.Fatal(err)
	}
	if _, err := receiver.command("device", "register"); err != nil {
		t.Fatal(err)
	}

	t.Run("direct", func(t *testing.T) {
		runP2PCase(t, sender, &receiver, input, filepath.Join(directory, "direct.bin"), "direct", "direct")
	})
	t.Run("relay", func(t *testing.T) {
		transfer := runP2PCase(t, sender, &receiver, input, filepath.Join(directory, "relay.bin"), "relay", "relay")
		pool, err := pgxpool.New(t.Context(), requiredEnv("PINELINK_DATABASE_URL"))
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		var entryID, spoolState string
		deadline := time.Now().Add(10 * time.Second)
		for {
			if err := pool.QueryRow(t.Context(), `
			SELECT e.id, v.state FROM transfer_requests tr
			JOIN file_entries e ON e.id=tr.entry_id
			JOIN file_versions v ON v.id=e.version_id WHERE tr.id=$1
		`, transfer.ID).Scan(&entryID, &spoolState); err != nil {
				t.Fatal(err)
			}
			if spoolState == "SEALED" || time.Now().After(deadline) {
				break
			}
			time.Sleep(50 * time.Millisecond)
		}
		if spoolState != "SEALED" {
			t.Fatalf("relay spool is %s", spoolState)
		}
		spoolOutput := filepath.Join(directory, "spool.bin")
		sender.download(entryID, spoolOutput, false)
		assertFileEquals(input, spoolOutput, t)
		// A failed optional cloud copy must not revoke an already valid delivery receipt.
		if _, err := pool.Exec(t.Context(), `UPDATE file_versions SET state='CORRUPT',version=version+1 WHERE id=(SELECT version_id FROM file_entries WHERE id=$1)`, entryID); err != nil {
			t.Fatal(err)
		}
		text, err := receiver.command("p2p", "progress", transfer.ID)
		if err != nil {
			t.Fatal(err)
		}
		var snapshot progressSnapshot
		if err = json.Unmarshal([]byte(text), &snapshot); err != nil {
			t.Fatal(err)
		}
		if !snapshot.Completion.Delivered || snapshot.Completion.Stored || snapshot.Spool.State != "FAILED" {
			t.Fatalf("cloud failure changed delivery: %s", text)
		}
	})
	t.Run("auto-fallback", func(t *testing.T) {
		runP2PCase(t, sender, &receiver, input, filepath.Join(directory, "auto.bin"), "auto", "relay")
	})
	t.Run("mid-transfer-fallback", func(t *testing.T) {
		largeInput := filepath.Join(directory, "large.bin")
		large := make([]byte, 4<<20)
		for index := range large {
			large[index] = byte(index * 7)
		}
		if err := os.WriteFile(largeInput, large, 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("PINELINK_P2P_FAIL_AFTER_DIRECT", "65536")
		transfer := runP2PCase(t, sender, &receiver, largeInput, filepath.Join(directory, "mid-transfer.bin"), "auto", "auto")
		pool, err := pgxpool.New(t.Context(), requiredEnv("PINELINK_DATABASE_URL"))
		if err != nil {
			t.Fatal(err)
		}
		defer pool.Close()
		var directSignals, relaySignals int
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM peer_signals WHERE transfer_id=$1 AND channel='direct-offer'`, transfer.ID).Scan(&directSignals); err != nil {
			t.Fatal(err)
		}
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM peer_signals WHERE transfer_id=$1 AND channel='relay-uplink-offer'`, transfer.ID).Scan(&relaySignals); err != nil {
			t.Fatal(err)
		}
		if directSignals == 0 || relaySignals == 0 {
			t.Fatalf("mid-transfer fallback did not use both paths: direct=%d relay=%d", directSignals, relaySignals)
		}
	})
}

func runP2PCase(t *testing.T, sender, receiver *harness, input, output, senderPath, receiverPath string) lifecycleTransfer {
	senderProcess := sender.start("p2p", "send", input, "--to-owner", receiver.owner, "--to-device", receiver.device,
		"--path", senderPath, "--direct-timeout", "10s")
	transfer := receiver.waitP2PRequest(t)
	receiver.transferAction(t, "accept", transfer.ID)
	receiverProcess := receiver.start("p2p", "receive", transfer.ID, output, "--path", receiverPath, "--direct-timeout", "10s")
	if text, err := watchOutput(receiverProcess); err != nil {
		senderText, _ := watchOutput(senderProcess)
		t.Fatalf("P2P receive failed: %v: %s; sender: %s", err, text, senderText)
	}
	if text, err := watchOutput(senderProcess); err != nil {
		t.Fatalf("P2P send failed: %v: %s", err, text)
	}
	assertFileEquals(input, output, t)
	text, err := receiver.command("p2p", "progress", transfer.ID)
	if err != nil {
		t.Fatal(err)
	}
	var snapshot progressSnapshot
	if err = json.Unmarshal([]byte(text), &snapshot); err != nil {
		t.Fatal(err)
	}
	if !snapshot.Completion.Delivered || snapshot.Delivery.State != "DELIVERED" || snapshot.Delivery.Acknowledged == 0 || snapshot.Delivery.SampledAt == nil {
		t.Fatalf("real delivery snapshot: %s", text)
	}
	wantRoute := "DIRECT"
	if senderPath == "relay" || receiverPath == "relay" || senderPath == "auto" {
		wantRoute = "RELAY"
	}
	if snapshot.Route.Kind != wantRoute {
		t.Fatalf("route=%s want %s: %s", snapshot.Route.Kind, wantRoute, text)
	}
	return transfer
}

func (h *harness) waitP2PRequest(t *testing.T) lifecycleTransfer {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		output, err := h.command("transfers", "--role", "recipient", "--state", "PENDING")
		if err != nil {
			t.Fatal(err)
		}
		var list lifecycleTransferList
		if err := json.Unmarshal([]byte(output), &list); err != nil {
			t.Fatal(err)
		}
		for _, transfer := range list.Transfers {
			if transfer.DeliveryMode == "ONLINE" {
				return transfer
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("no pending P2P request appeared")
	return lifecycleTransfer{}
}
