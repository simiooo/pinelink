package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Cut only the test receiver's first upgraded connection, after a complete
// ciphertext frame and part of the next. Production transport is unchanged.
type interruptedRelay struct {
	io.ReadWriteCloser
	remaining int64
}

func (r *interruptedRelay) Read(data []byte) (int, error) {
	if r.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(data)) > r.remaining {
		data = data[:r.remaining]
	}
	n, err := r.ReadWriteCloser.Read(data)
	r.remaining -= int64(n)
	return n, err
}

func TestP2PRelaySocketPauseAndResume(t *testing.T) {
	if os.Getenv("PINELINK_RELAY_TRANSPORT") != "websocket" {
		t.Skip("requires the WebSocket relay")
	}
	id := uuid.NewString()
	sender := &harness{t: t, server: requiredEnv("PINELINK_TEST_SERVER"), cli: requiredEnv("PINELINK_TEST_CLI"), scope: "recovery-" + id, domain: "recovery", owner: "sender-" + id, device: "sender", dedupSecret: "recovery-fixture"}
	receiver := *sender
	receiver.owner = "receiver-" + id
	receiver.device = "receiver"
	for _, actor := range []*harness{sender, &receiver} {
		if _, err := actor.command("device", "register"); err != nil {
			t.Fatal(err)
		}
	}
	directory := t.TempDir()
	input := filepath.Join(directory, "input.bin")
	output := filepath.Join(directory, "output.bin")
	data := make([]byte, 300_000)
	for i := range data {
		data[i] = byte(i * 47)
	}
	if err := os.WriteFile(input, data, 0o600); err != nil {
		t.Fatal(err)
	}
	sending := sender.start("p2p", "send", input, "--to-owner", receiver.owner, "--to-device", receiver.device, "--path", "relay")
	request := receiver.waitP2PRequest(t)
	if text, err := watchOutput(sending); err != nil {
		t.Fatal("preupload", text, err)
	}
	receiver.transferAction(t, "accept", request.ID)
	receiver.taskAction(t, "pause", request.ID)
	target, err := url.Parse(sender.server)
	if err != nil {
		t.Fatal(err)
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	director := proxy.Director
	proxy.Director = func(r *http.Request) { director(r); r.Host = target.Host }
	var interrupted atomic.Bool
	opened := make(chan struct{}, 1)
	proxy.ModifyResponse = func(response *http.Response) error {
		if response.StatusCode == http.StatusSwitchingProtocols && response.Request.URL.Query().Get("role") == "receiver" && interrupted.CompareAndSwap(false, true) {
			connection, ok := response.Body.(io.ReadWriteCloser)
			if !ok {
				t.Error("upgrade body is not duplex")
				return io.ErrUnexpectedEOF
			}
			response.Body = &interruptedRelay{ReadWriteCloser: connection, remaining: 100 << 10}
			opened <- struct{}{}
		}
		return nil
	}
	fault := httptest.NewServer(proxy)
	defer fault.Close()
	faultReceiver := receiver
	faultReceiver.server = fault.URL
	receiving := faultReceiver.start("p2p", "receive", request.ID, output, "--path", "relay")
	select {
	case <-opened:
	case <-time.After(15 * time.Second):
		_ = receiving.Process.Kill()
		t.Fatal("receiver did not open the paused relay")
	}
	time.Sleep(250 * time.Millisecond)
	if info, err := os.Stat(output + ".p2p"); err == nil && info.Size() != 0 {
		_ = receiving.Process.Kill()
		t.Fatal("paused relay wrote ciphertext")
	}
	receiver.taskAction(t, "resume", request.ID)
	if text, err := watchOutput(receiving); err == nil {
		t.Fatal("injected disconnect unexpectedly completed", text)
	}
	partial, err := os.Stat(output + ".p2p")
	if err != nil || partial.Size() <= 0 || partial.Size() >= int64(len(data)) || partial.Size()%(64<<10) != 0 {
		t.Fatal("missing bounded durable checkpoint", partial, err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatal("partial ciphertext published as completed plaintext")
	}
	text, err := receiver.command("p2p", "progress", request.ID)
	if err != nil {
		t.Fatal(err)
	}
	var before progressSnapshot
	if err = json.Unmarshal([]byte(text), &before); err != nil {
		t.Fatal(err)
	}
	if before.Completion.Delivered || before.Delivery.Acknowledged != partial.Size() {
		t.Fatal("disconnect checkpoint is not authoritative", text)
	}
	resumed := faultReceiver.start("p2p", "receive", request.ID, output, "--path", "relay")
	if text, err := watchOutput(resumed); err != nil {
		t.Fatal("resume", text, err)
	}
	assertFileEquals(input, output, t)
	text, err = receiver.command("p2p", "progress", request.ID)
	if err != nil {
		t.Fatal(err)
	}
	var after progressSnapshot
	if err = json.Unmarshal([]byte(text), &after); err != nil {
		t.Fatal(err)
	}
	if !after.Completion.Delivered || !after.Completion.Stored || after.Route.Kind != "RELAY" {
		t.Fatal("resumed transfer lacks its verified receipt", text)
	}
	for _, actor := range []*harness{sender, &receiver} {
		if _, err := actor.command("device", "revoke"); err != nil {
			t.Fatal(err)
		}
	}
}
