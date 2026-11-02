package lan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

type testReceiver struct {
	server   *Server
	peer     Peer
	identity *secure.Identity
	cfg      Config
	cancel   context.CancelFunc
	done     chan error
}

func startReceiver(t *testing.T, identity *secure.Identity, cfg Config) *testReceiver {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	cfg.Address = "https://" + listener.Addr().String()
	s, err := NewServer(identity, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Prepare(listener); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	r := &testReceiver{server: s, peer: s.Peer(), identity: identity, cfg: cfg, cancel: cancel, done: make(chan error, 1)}
	go func() { r.done <- s.Serve(ctx, listener) }()
	t.Cleanup(func() {
		cancel()
		s.Close()
		select {
		case err := <-r.done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("local receiver did not stop")
		}
	})
	return r
}
func identities(t *testing.T) (*secure.Identity, *secure.Identity) {
	t.Helper()
	t.Setenv("PINELINK_CONFIG_HOME", t.TempDir())
	receiver, err := secure.LoadIdentity("owner", "receiver")
	if err != nil {
		t.Fatal(err)
	}
	sender, err := secure.LoadIdentity("owner", "sender")
	if err != nil {
		t.Fatal(err)
	}
	return receiver, sender
}
func pairedClient(t *testing.T, r *testReceiver, sender *secure.Identity) (*Client, *Client) {
	t.Helper()
	source, err := NewClient(sender, r.peer, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(source.Close)
	admin, err := NewClient(r.identity, r.peer, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	invite, err := r.server.Invite()
	if err != nil {
		t.Fatal(err)
	}
	code, err := invite.Code()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseInvitation(code)
	if err != nil {
		t.Fatal(err)
	}
	pair, err := source.Pair(t.Context(), parsed, "Source laptop")
	if err != nil {
		t.Fatal(err)
	}
	if pair.State != "APPROVED" {
		pair, err = admin.PairAction(t.Context(), pair.ID, "approve")
		if err != nil {
			t.Fatal(err)
		}
	}
	if pair.State != "APPROVED" {
		t.Fatal("pair was not approved")
	}
	if err = source.Remember(); err != nil {
		t.Fatal(err)
	}
	return source, admin
}
func inputFile(t *testing.T, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "Design source.fig")
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestOnePairEnablesBothDirectionsAndHistoryStaysOffline(t *testing.T) {
	bIdentity, aIdentity := identities(t)
	a := startReceiver(t, aIdentity, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "A", AutoAccept: true})
	b := startReceiver(t, bIdentity, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "B", AutoAccept: true, AutoPair: true})
	aToB, err := NewClient(aIdentity, b.peer, a.cfg.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer aToB.Close()
	invite, _ := b.server.Invite()
	pair, err := aToB.Pair(t.Context(), invite, "A")
	if err != nil || pair.State != "APPROVED" {
		t.Fatal(pair.State, err)
	}
	if err = aToB.Remember(); err != nil {
		t.Fatal(err)
	}
	contact, err := ReadContact(b.cfg.Directory, a.peer.ID)
	if err != nil || contact.Address != a.peer.Address {
		t.Fatal("pair lost the reciprocal endpoint", err)
	}
	bToA, err := NewClient(bIdentity, contact, b.cfg.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer bToA.Close()
	var outbound Transfer
	for _, client := range []*Client{aToB, bToA} {
		prepared, err := client.PrepareFile(t.Context(), inputFile(t, []byte("both directions, one pairing")))
		if err != nil {
			t.Fatal(err)
		}
		completed, err := client.Send(t.Context(), prepared.ID, nil)
		if err != nil || completed.State != "COMPLETED" {
			t.Fatal(completed.State, err)
		}
		if client == aToB {
			outbound = completed
		}
	}
	admin, err := NewClient(aIdentity, a.peer, a.cfg.Directory)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err = admin.Forget(t.Context(), b.peer.ID); err != nil {
		t.Fatal("live receiver did not observe reciprocal trust", err)
	}
	if _, err = bToA.Transfers(t.Context()); err == nil {
		t.Fatal("forgotten peer retained its live session")
	}
	if _, err = ReadContact(a.cfg.Directory, b.peer.ID); err == nil {
		t.Fatal("forgotten outbound contact remained usable")
	}
	b.cancel()
	b.server.Close()
	if _, err = aToB.Send(t.Context(), outbound.ID, nil); err != nil {
		t.Fatal("completed delivery required the disconnected recipient", err)
	}
	history, err := History(aIdentity, a.cfg.Directory)
	if err != nil || len(history) != 2 {
		t.Fatal("offline history lost one direction", len(history), err)
	}
	for _, item := range history {
		if item.State != "COMPLETED" || item.Receipt == nil || len(item.Envelope) != 0 {
			t.Fatal("invalid public history snapshot")
		}
	}
}

func TestChunkAcknowledgementAndCorruptCommit(t *testing.T) {
	for _, corrupt := range []bool{false, true} {
		t.Run(fmt.Sprint(corrupt), func(t *testing.T) {
			receiver, sender := identities(t)
			r := startReceiver(t, receiver, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver", AutoAccept: true})
			source, _ := pairedClient(t, r, sender)
			prepared, err := source.PrepareFile(t.Context(), inputFile(t, bytes.Repeat([]byte("abc"), 30_000)))
			if err != nil {
				t.Fatal(err)
			}
			var saved sentRecord
			if err = readJSON(source.root, sentName(prepared.ID), &saved); err != nil {
				t.Fatal(err)
			}
			ciphertext, err := fs.ReadFile(source.root.FS(), saved.CipherName)
			if err != nil {
				t.Fatal(err)
			}
			for offset := 0; offset < len(ciphertext); offset += ChunkSize {
				path := fmt.Sprintf("transfers/%s/chunks/%d", prepared.ID, offset/ChunkSize)
				block := ciphertext[offset:min(offset+ChunkSize, len(ciphertext))]
				if err = source.request(t.Context(), "PUT", path, block, &prepared); err != nil {
					t.Fatal(err)
				}
				if offset == 0 {
					ack, revision := prepared.Acknowledged, prepared.Revision
					if err = source.request(t.Context(), "PUT", path, block, &prepared); err != nil || prepared.Acknowledged != ack || prepared.Revision != revision {
						t.Fatal("duplicate advanced durable progress", err)
					}
					part, e := r.server.root.OpenFile(partName(prepared.ID), os.O_WRONLY|os.O_APPEND, 0)
					if e != nil {
						t.Fatal(e)
					}
					_, e = part.Write([]byte("uncheckpointed crash tail"))
					part.Close()
					if e != nil {
						t.Fatal(e)
					}
				}
			}
			if corrupt {
				part, e := r.server.root.OpenFile(partName(prepared.ID), os.O_WRONLY, 0)
				if e != nil {
					t.Fatal(e)
				}
				_, e = part.WriteAt([]byte{ciphertext[0] ^ 1}, 0)
				part.Close()
				if e != nil {
					t.Fatal(e)
				}
			}
			completed, err := source.Send(t.Context(), prepared.ID, nil)
			if corrupt {
				if err == nil || completed.State != "FAILED" || completed.Receipt != nil {
					t.Fatal("corrupt ciphertext produced a delivery", completed.State, err)
				}
				files, e := os.ReadDir(r.cfg.ReceiveDirectory)
				if e != nil || len(files) != 0 {
					t.Fatal("corrupt transfer left a received file", e)
				}
			} else if err != nil || completed.State != "COMPLETED" {
				t.Fatal("checkpoint tail was not repaired", err)
			}
		})
	}
}

func TestLocalDeadlineExpiresWithoutFurtherRequests(t *testing.T) {
	receiver, sender := identities(t)
	r := startReceiver(t, receiver, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver", AutoAccept: true, TransferTTL: time.Second})
	source, _ := pairedClient(t, r, sender)
	prepared, err := source.PrepareFile(t.Context(), inputFile(t, []byte("expire without polling")))
	if err != nil {
		t.Fatal(err)
	}
	record, err := r.server.get(prepared.ID)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		record.mu.Lock()
		state := record.t.State
		record.mu.Unlock()
		if state == "EXPIRED" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("idle transfer did not expire")
}

func TestExpiredTransferRejectsActionsWhenJournalFails(t *testing.T) {
	receiver, sender := identities(t)
	r := startReceiver(t, receiver, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver"})
	source, _ := pairedClient(t, r, sender)
	prepared, err := source.PrepareFile(t.Context(), inputFile(t, []byte("expired")))
	if err != nil {
		t.Fatal(err)
	}
	record, _ := r.server.get(prepared.ID)
	record.mu.Lock()
	record.t.ExpiresAt = time.Now().Add(-time.Second)
	record.mu.Unlock()
	if err = r.server.root.Close(); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	r.server.action(response, record, receiver.LocalID(), "accept", prepared.Version)
	if response.Code >= 200 && response.Code < 300 {
		t.Fatal("failed expiry journal allowed a state change")
	}
	record.mu.Lock()
	defer record.mu.Unlock()
	if record.t.State != "PENDING" || record.t.Version != prepared.Version {
		t.Fatal("failed journal changed in-memory authority")
	}
}

func TestLongTransferSessionRotationIsBounded(t *testing.T) {
	receiver, sender := identities(t)
	r := startReceiver(t, receiver, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver"})
	source, _ := pairedClient(t, r, sender)
	for index := 0; index < 300; index++ {
		source.mu.Lock()
		source.token = ""
		source.mu.Unlock()
		if _, err := source.session(t.Context()); err != nil {
			t.Fatal("session rotation exhausted the receiver", err)
		}
	}
	r.server.mu.Lock()
	defer r.server.mu.Unlock()
	count := 0
	for _, session := range r.server.sessions {
		if session.peerID == sender.LocalID() {
			count++
		}
	}
	if count > 16 {
		t.Fatal("old sessions accumulated without bound")
	}
}

func TestPaginatedLocalHistoryKeepsPeerIsolation(t *testing.T) {
	receiver, sender := identities(t)
	r := startReceiver(t, receiver, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver"})
	source, admin := pairedClient(t, r, sender)
	r.server.mu.Lock()
	for index := 0; index < 425; index++ {
		id, peer := uuid.NewString(), sender.LocalID()
		if index == 424 {
			peer = receiver.LocalID()
		}
		r.server.transfers[id] = newRecord(Transfer{Offer: Offer{ID: id}, PeerID: peer, State: "CANCELLED"})
	}
	r.server.mu.Unlock()
	visible, err := source.Transfers(t.Context())
	if err != nil || len(visible) != 424 {
		t.Fatal("paged participant history incomplete", len(visible), err)
	}
	all, err := admin.Transfers(t.Context())
	if err != nil || len(all) != 425 {
		t.Fatal("paged owner history incomplete", len(all), err)
	}
}

func TestOfflineDeliveryAndSignedReceipt(t *testing.T) {
	for _, size := range []int{0, 300_000} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			receiver, sender := identities(t)
			r := startReceiver(t, receiver, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver", AutoAccept: true})
			source, _ := pairedClient(t, r, sender)
			data := bytes.Repeat([]byte{71}, size)
			input := inputFile(t, data)
			prepared, err := source.PrepareFile(t.Context(), input)
			if err != nil {
				t.Fatal(err)
			}
			completed, err := source.Send(t.Context(), prepared.ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			if completed.State != "COMPLETED" || completed.Receipt == nil || completed.SpoolState != "NOT_REQUIRED" {
				t.Fatalf("wrong completion: %+v", completed)
			}
			got, err := os.ReadFile(filepath.Join(r.cfg.ReceiveDirectory, completed.OutputName))
			if err != nil || !bytes.Equal(got, data) {
				t.Fatal("received file differs", err)
			}
			if _, err = source.Send(t.Context(), prepared.ID, nil); err != nil {
				t.Fatal("duplicate send lost its receipt", err)
			}
			var saved sentRecord
			if err = readJSON(source.root, sentName(prepared.ID), &saved); err != nil {
				t.Fatal(err)
			}
			if err = saved.Status.Receipt.Verify(r.peer, saved.Offer, sender.LocalID()); err != nil {
				t.Fatal(err)
			}
			if _, err = source.root.Stat(saved.CipherName); !os.IsNotExist(err) {
				t.Fatal("completed sender retained ciphertext cache")
			}
		})
	}
}

func TestPairingConsentKeyPinAndOneTimeInvite(t *testing.T) {
	receiver, sender := identities(t)
	r := startReceiver(t, receiver, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver"})
	source, err := NewClient(sender, r.peer, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	if _, err = source.Transfers(t.Context()); err == nil {
		t.Fatal("unpaired peer accessed control")
	}
	invite, err := r.server.Invite()
	if err != nil {
		t.Fatal(err)
	}
	pair, err := source.Pair(t.Context(), invite, "Source")
	if err != nil || pair.State != "WAITING" {
		t.Fatal("pairing skipped confirmation", pair, err)
	}
	if _, err = source.session(t.Context()); err == nil {
		t.Fatal("pending peer got a session")
	}
	other, err := secure.LoadIdentity("other", "device")
	if err != nil {
		t.Fatal(err)
	}
	attacker, err := NewClient(other, r.peer, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer attacker.Close()
	if _, err = attacker.Pair(t.Context(), invite, "Other"); err == nil {
		t.Fatal("one-time invitation paired another device")
	}
	wrong := r.peer
	wrong.ID = other.LocalID()
	wrong.SigningKey = other.SigningPublicKey()
	wrong.EncryptionKey = other.PublicKey()
	pinned, err := NewClient(sender, wrong, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.Close()
	if _, err = pinned.Info(t.Context()); err == nil {
		t.Fatal("wrong pinned certificate was accepted")
	}
	admin, err := NewClient(receiver, r.peer, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if _, err = admin.PairAction(t.Context(), pair.ID, "approve"); err != nil {
		t.Fatal(err)
	}
	if _, err = source.session(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestLocalConsentPauseCancellationAndReplay(t *testing.T) {
	receiver, sender := identities(t)
	r := startReceiver(t, receiver, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver"})
	source, admin := pairedClient(t, r, sender)
	prepared, err := source.PrepareFile(t.Context(), inputFile(t, []byte("secret")))
	if err != nil {
		t.Fatal(err)
	}
	if prepared.State != "PENDING" {
		t.Fatal("file consent was skipped")
	}
	if _, err = source.Action(t.Context(), prepared.ID, "accept", prepared.Version); err == nil {
		t.Fatal("sender accepted its own transfer")
	}
	if err = source.request(t.Context(), "PUT", "transfers/"+prepared.ID+"/chunks/0", []byte("bad"), nil); err == nil {
		t.Fatal("bytes accepted before consent")
	}
	accepted, err := admin.Action(t.Context(), prepared.ID, "accept", prepared.Version)
	if err != nil {
		t.Fatal(err)
	}
	paused, err := source.Action(t.Context(), prepared.ID, "pause", accepted.Version)
	if err != nil {
		t.Fatal(err)
	}
	if paused.State != "PAUSED" {
		t.Fatal("sender pause did not reach the local authority")
	}
	resumed, err := admin.Action(t.Context(), prepared.ID, "resume", paused.Version)
	if err != nil {
		t.Fatal(err)
	}
	cancelled, err := source.Action(t.Context(), prepared.ID, "cancel", resumed.Version)
	if err != nil {
		t.Fatal(err)
	}
	if cancelled.State != "CANCELLED" {
		t.Fatal("cancel failed")
	}
	if _, err = source.Send(t.Context(), prepared.ID, nil); err == nil {
		t.Fatal("cancelled attempt resumed")
	}
	token, err := admin.session(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	nonce := randomToken()
	body := []byte{}
	send := func() int {
		req, _ := http.NewRequestWithContext(t.Context(), "POST", admin.address+"/local/v1/invites", bytes.NewReader(body))
		req.Header.Set("X-Pinelink-Local-Session", token)
		req.Header.Set("X-Pinelink-Local-Nonce", nonce)
		req.Header.Set("X-Pinelink-Local-Signature", receiver.Sign(requestProof(token, nonce, "POST", req.URL.RequestURI(), body)))
		response, err := admin.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	if send() != 200 || send() != 409 {
		t.Fatal("mutation replay was not rejected")
	}
}

func TestLocalReceiverRestartAndForget(t *testing.T) {
	receiver, sender := identities(t)
	cfg := Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver", AutoAccept: true}
	r := startReceiver(t, receiver, cfg)
	source, _ := pairedClient(t, r, sender)
	prepared, err := source.PrepareFile(t.Context(), inputFile(t, bytes.Repeat([]byte{9}, 120_000)))
	if err != nil {
		t.Fatal(err)
	}
	var saved sentRecord
	if err = readJSON(source.root, sentName(prepared.ID), &saved); err != nil {
		t.Fatal(err)
	}
	file, err := source.root.Open(saved.CipherName)
	if err != nil {
		t.Fatal(err)
	}
	block := make([]byte, ChunkSize)
	_, err = file.ReadAt(block, 0)
	file.Close()
	if err != nil {
		t.Fatal(err)
	}
	if err = source.request(t.Context(), "PUT", "transfers/"+prepared.ID+"/chunks/0", block, &prepared); err != nil {
		t.Fatal(err)
	}
	oldToken := source.token
	r.cancel()
	r.server.Close()
	listener, err := net.Listen("tcp", r.peer.Address[len("https://"):])
	if err != nil {
		t.Fatal(err)
	}
	cfg.Address = r.peer.Address
	replacement, err := NewServer(receiver, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer replacement.Close()
	if err = replacement.Prepare(listener); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go replacement.Serve(ctx, listener)
	if err = source.raw(t.Context(), "GET", "transfers", nil, oldToken, nil); err == nil {
		t.Fatal("pre-restart session remained valid")
	}
	completed, err := source.Send(t.Context(), prepared.ID, nil)
	if err != nil {
		t.Fatal("checkpoint did not resume after receiver restart", err)
	}
	if completed.State != "COMPLETED" {
		t.Fatal("not delivered")
	}
	admin, err := NewClient(receiver, r.peer, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err = admin.Forget(t.Context(), sender.LocalID()); err != nil {
		t.Fatal(err)
	}
	if _, err = source.Transfers(t.Context()); err == nil {
		t.Fatal("forgotten peer kept access")
	}
}

func TestLocalExpiryTamperAndOutputNames(t *testing.T) {
	receiver, sender := identities(t)
	r := startReceiver(t, receiver, Config{Directory: t.TempDir(), ReceiveDirectory: t.TempDir(), Name: "Receiver", AutoAccept: true})
	source, _ := pairedClient(t, r, sender)
	prepared, err := source.PrepareFile(t.Context(), inputFile(t, []byte("integrity")))
	if err != nil {
		t.Fatal(err)
	}
	rec, _ := r.server.get(prepared.ID)
	rec.mu.Lock()
	rec.t.ExpiresAt = time.Now().Add(-time.Second)
	rec.mu.Unlock()
	if _, err = source.Send(t.Context(), prepared.ID, nil); err == nil {
		t.Fatal("expired local transfer resumed")
	}
	var saved sentRecord
	if err = readJSON(source.root, sentName(prepared.ID), &saved); err != nil {
		t.Fatal(err)
	}
	copied := saved.Offer
	copied.ID = "../../outside"
	body, _ := json.Marshal(copied)
	if err = source.request(t.Context(), "POST", "transfers", body, nil); err == nil {
		t.Fatal("path traversal id accepted")
	}
	name := safeName("../../CON\\secret.txt", prepared.ID)
	if filepath.Base(name) != name {
		t.Fatal("output escaped selected directory")
	}
	if len(safeName(string(bytes.Repeat([]byte("长"), 120))+".txt", prepared.ID)) > 255 {
		t.Fatal("output name exceeds filesystem bound")
	}
	var status *StatusError
	if _, err = ReadContact(t.TempDir(), "../../x"); err == nil || errors.As(err, &status) {
		t.Fatal("invalid peer id was not rejected")
	}
}
