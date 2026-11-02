package service

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	rp "github.com/simiooo/pinelink/protocol/replication"
)

type replicaTestActor struct {
	owner, device, root, public, age string
	key                              ed25519.PrivateKey
}

func TestReplicationFirstRegistrationRequiresProofOfPossession(t *testing.T) {
	s := replicaService(t)
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	request := DeviceRequest{OwnerID: "first-owner", DeviceID: "first-device", PublicKey: "age1test", SigningPublicKey: base64.StdEncoding.EncodeToString(public)}
	body, _ := json.Marshal(request)
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	verify := func(signature string, input []byte) error {
		return s.VerifyDevice(t.Context(), request.OwnerID, request.DeviceID, timestamp, "POST", "/v1/devices/register", input, signature, true)
	}
	if err = verify("", body); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("unsigned first registration accepted", err)
	}
	digest := sha256.Sum256(body)
	proof := []byte("POST\n/v1/devices/register\n" + timestamp + "\n" + hex.EncodeToString(digest[:]))
	signature := base64.StdEncoding.EncodeToString(ed25519.Sign(private, proof))
	if err = verify(signature, body); err != nil {
		t.Fatal("valid first registration refused", err)
	}
	request.DeviceID = "other-device"
	changed, _ := json.Marshal(request)
	if err = verify(signature, changed); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("registration proof allowed another identity", err)
	}
}

func replicaActor(t *testing.T, s *Service, label string) replicaTestActor {
	t.Helper()
	public, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	a := replicaTestActor{owner: "owner-" + label, device: "device-" + label, public: base64.StdEncoding.EncodeToString(public), age: "age1test-" + label, key: key}
	a.root, _ = rp.Root(a.public)
	if _, e = s.RegisterDevice(t.Context(), DeviceRequest{OwnerID: a.owner, DeviceID: a.device, PublicKey: a.age, SigningPublicKey: a.public}); e != nil {
		t.Fatal(e)
	}
	if _, e = s.BindReplication(t.Context(), a.owner, a.device, rp.Binding{Root: a.root, SigningKey: a.public, EncryptionKey: a.age}); e != nil {
		t.Fatal(e)
	}
	return a
}
func replicaService(t *testing.T) *Service {
	s, _, _ := testStorage(t)
	_, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	if e = s.SetReplicationKey(key); e != nil {
		t.Fatal(e)
	}
	return s
}
func replicaWire(a, b replicaTestActor, stream, id string, sequence, version int64) rp.Envelope {
	recipients := []string{a.root, b.root}
	slices.Sort(recipients)
	e := rp.Envelope{Version: rp.Version, EventID: uuid.NewString(), SourceRoot: a.root, StreamID: stream, Sequence: sequence, AggregateID: id, AggregateVersion: version, Authority: "LOCAL_DEVICE", AuthorityRoot: b.root, Kind: "SENT_CHANGED", PeerRoot: b.root, Recipients: recipients, SigningKey: a.public, Ciphertext: []byte("opaque encrypted metadata")}
	e.Signature = rp.Sign(a.key, e.Proof())
	return e
}
func replicaRoster(t *testing.T, s *Service, a, b replicaTestActor, version int64, enabled bool) rp.Roster {
	r := rp.Roster{OperationID: uuid.NewString(), ExpectedVersion: version, Peers: []rp.Peer{}}
	if enabled {
		r.Peers = append(r.Peers, rp.Peer{Root: b.root, SigningKey: b.public, EncryptionKey: b.age})
	}
	if _, e := s.UpdateReplicationRoster(t.Context(), a.owner, a.device, r); e != nil {
		t.Fatal(e)
	}
	return r
}
func replicaRead(t *testing.T, s *Service, a replicaTestActor, after int64) rp.Page {
	p, e := s.ReadReplication(t.Context(), a.owner, a.device, "", after, 100, false, -1, "")
	if e != nil {
		t.Fatal(e)
	}
	if p.Validate(s.replicationPublic(), a.root) != nil {
		t.Fatal("invalid server-signed page")
	}
	return p
}

func TestReplicationIdempotenceMutualConsentAndRevocation(t *testing.T) {
	s := replicaService(t)
	a, b := replicaActor(t, s, "a"), replicaActor(t, s, "b")
	stream, id := uuid.NewString(), uuid.NewString()
	e := replicaWire(a, b, stream, id, 2, 1)
	batch := rp.Upload{SourceRoot: a.root, StreamID: stream, After: 0, Through: 3, Events: []rp.Envelope{e}}
	for i := 0; i < 2; i++ {
		ack, err := s.UploadReplication(t.Context(), a.owner, a.device, batch)
		if err != nil || ack.Through != 3 || !rp.Verify(s.replicationPublic(), ack.Proof(), ack.Signature) {
			t.Fatal("upload retry failed", err)
		}
	}
	if p := replicaRead(t, s, a, 0); len(p.Items) != 1 {
		t.Fatal("own feed duplicate", len(p.Items))
	}
	if p := replicaRead(t, s, b, 0); len(p.Items) != 0 {
		t.Fatal("one party shared without recipient consent")
	}
	replicaRoster(t, s, a, b, 0, true)
	if e := s.ProjectReplication(t.Context()); e != nil {
		t.Fatal(e)
	}
	if p := replicaRead(t, s, b, 0); len(p.Items) != 0 {
		t.Fatal("unilateral link granted access")
	}
	original := replicaRoster(t, s, b, a, 0, true)
	if e := s.ProjectReplication(t.Context()); e != nil {
		t.Fatal(e)
	}
	first := replicaRead(t, s, b, 0)
	if len(first.Items) != 1 {
		t.Fatal("pending history did not backfill", len(first.Items))
	}
	replicaRoster(t, s, b, a, 1, false)
	if p := replicaRead(t, s, b, 0); len(p.Items) != 0 {
		t.Fatal("revoked link remained readable")
	}
	if _, err := s.UpdateReplicationRoster(t.Context(), b.owner, b.device, original); err != nil {
		t.Fatal("idempotent old operation failed", err)
	}
	if p := replicaRead(t, s, b, 0); len(p.Items) != 0 {
		t.Fatal("replayed grant reversed revocation")
	}
	replicaRoster(t, s, b, a, 2, true)
	if err := s.ProjectReplication(t.Context()); err != nil {
		t.Fatal(err)
	}
	if p := replicaRead(t, s, b, first.Through); len(p.Items) != 1 {
		t.Fatal("renewed grant did not replay to a new delivery position")
	}
	changed := e
	changed.Ciphertext = []byte("different ciphertext")
	changed.Signature = rp.Sign(a.key, changed.Proof())
	batch.Events[0] = changed
	if _, err := s.UploadReplication(t.Context(), a.owner, a.device, batch); !errors.Is(err, ErrConflict) {
		t.Fatal("same ID changed its bytes", err)
	}
	if _, err := s.UploadReplication(t.Context(), b.owner, b.device, batch); !errors.Is(err, ErrUnauthorized) {
		t.Fatal("source impersonation accepted", err)
	}
}

func TestReplicationLateCommitCannotBeSkipped(t *testing.T) {
	s := replicaService(t)
	a := replicaActor(t, s, "late")
	first, e := s.db.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	defer first.Rollback(context.Background())
	id1, id2 := uuid.NewString(), uuid.NewString()
	payload := map[string]any{"owner_id": a.owner, "device_id": a.device, "state": "SEALED", "file_name": "private-project-name.fig", "encrypted_manifest": "must-not-replay"}
	if e = addEvent(t.Context(), first, "CLOUD_STORED", id1, 1, payload); e != nil {
		t.Fatal(e)
	}
	second, e := s.db.Begin(t.Context())
	if e != nil {
		t.Fatal(e)
	}
	if e = addEvent(t.Context(), second, "CLOUD_STORED", id2, 1, payload); e != nil {
		t.Fatal(e)
	}
	if e = second.Commit(t.Context()); e != nil {
		t.Fatal(e)
	}
	if e = s.ProjectReplication(t.Context()); e != nil {
		t.Fatal(e)
	}
	page := replicaRead(t, s, a, 0)
	if len(page.Items) != 1 || page.Items[0].Event.AggregateID != id2 {
		t.Fatal("wrong first committed event")
	}
	encoded, _ := json.Marshal(page)
	if strings.Contains(string(encoded), "private-project-name") || strings.Contains(string(encoded), "must-not-replay") {
		t.Fatal("private business metadata escaped into history")
	}
	if e = first.Commit(t.Context()); e != nil {
		t.Fatal(e)
	}
	if e = s.ProjectReplication(t.Context()); e != nil {
		t.Fatal(e)
	}
	page2 := replicaRead(t, s, a, page.Through)
	if len(page2.Items) != 1 || page2.Items[0].Event.AggregateID != id1 {
		t.Fatal("late commit was skipped")
	}
}

func TestReplicationConcurrentRetryAndRosterCAS(t *testing.T) {
	s := replicaService(t)
	a, b := replicaActor(t, s, "retry-a"), replicaActor(t, s, "retry-b")
	stream := uuid.NewString()
	event := replicaWire(a, b, stream, uuid.NewString(), 1, 1)
	batch := rp.Upload{SourceRoot: a.root, StreamID: stream, Through: 1, Events: []rp.Envelope{event}}
	gate := make(chan struct{})
	results := make(chan error, 12)
	for range 12 {
		go func() {
			<-gate
			ack, err := s.UploadReplication(t.Context(), a.owner, a.device, batch)
			if err == nil && (ack.Through != 1 || !rp.Verify(s.replicationPublic(), ack.Proof(), ack.Signature)) {
				err = errors.New("invalid concurrent retry acknowledgement")
			}
			results <- err
		}()
	}
	close(gate)
	for range 12 {
		if err := <-results; err != nil {
			t.Fatal(err)
		}
	}
	page := replicaRead(t, s, a, 0)
	if len(page.Items) != 1 || page.Items[0].Event.EventID != event.EventID {
		t.Fatal("concurrent retry duplicated or lost the event")
	}
	// Both tabs observed roster version zero. Exactly one distinct operation
	// may advance it; the loser must refresh rather than overwrite the winner.
	gate = make(chan struct{})
	for range 2 {
		operation := rp.Roster{OperationID: uuid.NewString(), ExpectedVersion: 0, Peers: []rp.Peer{}}
		go func() {
			<-gate
			_, err := s.UpdateReplicationRoster(t.Context(), a.owner, a.device, operation)
			results <- err
		}()
	}
	close(gate)
	accepted, conflicted := 0, 0
	for range 2 {
		err := <-results
		if err == nil {
			accepted++
		} else if errors.Is(err, ErrConflict) {
			conflicted++
		} else {
			t.Fatal(err)
		}
	}
	if accepted != 1 || conflicted != 1 {
		t.Fatalf("CAS accepted=%d conflicted=%d", accepted, conflicted)
	}
}

func TestReplicationSnapshotBoundAndIdentityIsolation(t *testing.T) {
	s := replicaService(t)
	a, b := replicaActor(t, s, "snapshot-a"), replicaActor(t, s, "snapshot-b")
	stream := uuid.NewString()
	id1, id2 := uuid.NewString(), uuid.NewString()
	e1, e2 := replicaWire(a, b, stream, id1, 1, 1), replicaWire(a, b, stream, id2, 2, 1)
	if _, e := s.UploadReplication(t.Context(), a.owner, a.device, rp.Upload{SourceRoot: a.root, StreamID: stream, Through: 2, Events: []rp.Envelope{e1, e2}}); e != nil {
		t.Fatal(e)
	}
	first, e := s.ReadReplication(t.Context(), a.owner, a.device, "", 0, 1, true, -1, "")
	if e != nil || !first.More || len(first.Items) != 1 {
		t.Fatal(first, e)
	}
	e3 := replicaWire(a, b, stream, id1, 3, 2)
	if _, e = s.UploadReplication(t.Context(), a.owner, a.device, rp.Upload{SourceRoot: a.root, StreamID: stream, After: 2, Through: 3, Events: []rp.Envelope{e3}}); e != nil {
		t.Fatal(e)
	}
	next, e := s.ReadReplication(t.Context(), a.owner, a.device, first.Epoch, 0, 1, true, first.Upper, first.NextKey)
	if e != nil || next.More || len(next.Items) != 1 {
		t.Fatal(next, e)
	}
	for _, p := range []rp.Page{first, next} {
		for _, d := range p.Items {
			if d.Event.AggregateVersion != 1 {
				t.Fatal("snapshot moved beyond its upper bound")
			}
		}
	}
	if p := replicaRead(t, s, a, first.Upper); len(p.Items) != 1 || p.Items[0].Event.AggregateVersion != 2 {
		t.Fatal("increment after snapshot missing")
	}
	if _, e = s.ReadReplication(t.Context(), a.owner, b.device, "", 0, 100, false, -1, ""); !errors.Is(e, ErrUnauthorized) {
		t.Fatal("owner label granted another device's history", e)
	}
	if _, e = s.ReadReplication(t.Context(), a.owner, a.device, uuid.NewString(), 0, 100, false, -1, ""); !errors.Is(e, ErrConflict) {
		t.Fatal("stale epoch was accepted", e)
	}
}
