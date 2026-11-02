// historyfixture generates ephemeral interoperability vectors for client tests.
// It accepts public keys only and never exports or persists a private key.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"time"

	"filippo.io/age"
	"github.com/google/uuid"
	"github.com/simiooo/pinelink/cli/internal/lan"
	"github.com/simiooo/pinelink/cli/internal/secure"
	rp "github.com/simiooo/pinelink/protocol/replication"
)

func main() {
	var input rp.Peer
	check(json.NewDecoder(os.Stdin).Decode(&input))
	check(input.Validate())
	sourcePublic, source, err := ed25519.GenerateKey(rand.Reader)
	check(err)
	serverPublic, server, err := ed25519.GenerateKey(rand.Reader)
	check(err)
	ageKey, err := age.GenerateX25519Identity()
	check(err)
	sourceKey, serverKey := base64.StdEncoding.EncodeToString(sourcePublic), base64.StdEncoding.EncodeToString(serverPublic)
	sourceRoot, _ := rp.Root(sourceKey)
	serverRoot, _ := rp.Root(serverKey)
	id, stream, epoch := uuid.NewString(), uuid.NewString(), uuid.NewString()
	fact, err := json.Marshal(map[string]any{"transfer_id": id, "peer_id": input.Root, "direction": "RECEIVE", "state": "PENDING", "state_version": 1, "file_name": "本地 <>&.fig", "plaintext_size": 3, "cipher_root": rp.Hash([]byte("fixture ciphertext")), "authority": "LOCAL_DEVICE", "authority_device_id": sourceRoot})
	check(err)
	ciphertext, err := secure.SealHistory(fact, []string{input.EncryptionKey, ageKey.Recipient().String()})
	check(err)
	recipients := []string{input.Root, sourceRoot}
	slices.Sort(recipients)
	local := rp.Envelope{Version: 1, EventID: uuid.NewString(), SourceRoot: sourceRoot, StreamID: stream, Sequence: 1, AggregateID: id, AggregateVersion: 1, Authority: "LOCAL_DEVICE", AuthorityRoot: sourceRoot, Kind: "TRANSFER_CHANGED", PeerRoot: input.Root, Recipients: recipients, SigningKey: sourceKey, Ciphertext: ciphertext}
	local.Signature = rp.Sign(source, local.Proof())
	serverFact, _ := json.Marshal(map[string]any{"payload": map[string]any{"state": "STORED", "fixture": "中文 <>&\u2028"}})
	central := rp.Envelope{Version: 1, EventID: uuid.NewString(), SourceRoot: serverRoot, StreamID: epoch, Sequence: 1, AggregateID: id, AggregateVersion: 1, Authority: "SERVER", AuthorityRoot: serverRoot, Kind: "SERVER_FIXTURE", Recipients: []string{input.Root}, SigningKey: serverKey, ServerPayload: serverFact}
	central.Signature = rp.Sign(server, central.Proof())
	page := rp.Page{Root: input.Root, Epoch: epoch, After: 0, Through: 2, Upper: 2, Items: []rp.Delivery{{Sequence: 1, Event: local}, {Sequence: 2, Event: central}}}
	page.Signature = rp.Sign(server, page.Proof())
	staleSnapshot := rp.Page{Root: input.Root, Epoch: epoch, Through: 0, Upper: 2, Snapshot: true, More: true, NextKey: rp.SnapshotKey(local), Items: []rp.Delivery{{Sequence: 1, Event: local}}}
	staleSnapshot.Signature = rp.Sign(server, staleSnapshot.Proof())
	unsafePage := rp.Page{Root: input.Root, Epoch: epoch, Through: 9007199254740993, Upper: 9007199254740993, Items: []rp.Delivery{}}
	unsafePage.Signature = rp.Sign(server, unsafePage.Proof())
	receipt := lan.Receipt{Version: 1, TransferID: id, SenderID: input.Root, RecipientID: sourceRoot, CipherRoot: rp.Hash([]byte("fixture ciphertext")), PlainRoot: rp.Hash([]byte("abc")), PlainSize: 3, DeliveredAt: time.Now().UTC()}
	receipt.Signature = rp.Sign(source, []byte(fmt.Sprintf("pinelink-receipt-v1\n%d\n%s\n%s\n%s\n%s\n%s\n%d\n%s", receipt.Version, receipt.TransferID, receipt.SenderID, receipt.RecipientID, receipt.CipherRoot, receipt.PlainRoot, receipt.PlainSize, receipt.DeliveredAt.Format(time.RFC3339Nano))))
	check(receipt.Verify(lan.Peer{ID: sourceRoot, SigningKey: sourceKey}, lan.Offer{ID: id, CipherRoot: receipt.CipherRoot}, input.Root))
	completion := func(r lan.Receipt) rp.Page {
		var completed map[string]any
		check(json.Unmarshal(fact, &completed))
		completed["state"], completed["state_version"], completed["receipt"] = "COMPLETED", 2, r
		plain, _ := json.Marshal(completed)
		cipher, e := secure.SealHistory(plain, []string{input.EncryptionKey, ageKey.Recipient().String()})
		check(e)
		packet := local
		packet.EventID = uuid.NewString()
		packet.Sequence = 2
		packet.AggregateVersion = 2
		packet.Ciphertext = cipher
		packet.Signature = rp.Sign(source, packet.Proof())
		p := rp.Page{Root: input.Root, Epoch: epoch, After: 2, Through: 3, Upper: 3, Items: []rp.Delivery{{Sequence: 3, Event: packet}}}
		p.Signature = rp.Sign(server, p.Proof())
		return p
	}
	validCompletion := completion(receipt)
	receipt.Signature = base64.StdEncoding.EncodeToString(make([]byte, 64))
	invalidCompletion := completion(receipt)
	check(json.NewEncoder(os.Stdout).Encode(map[string]any{"pin": serverKey, "peer": rp.Peer{Root: sourceRoot, SigningKey: sourceKey, EncryptionKey: ageKey.Recipient().String()}, "page": page, "older_snapshot": staleSnapshot, "unsafe_page": unsafePage, "completion_page": validCompletion, "bad_receipt_page": invalidCompletion}))
}
func check(err error) {
	if err != nil {
		panic(err)
	}
}
