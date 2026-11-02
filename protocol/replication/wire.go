package replication

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
)

const Version = 1
const MaxBatch = 100
const MaxCiphertext = 64 << 10

var ErrInvalid = errors.New("invalid replication message")
var ErrConflict = errors.New("replication cursor or event conflict")

func Root(publicKey string) (string, error) {
	key, err := base64.StdEncoding.DecodeString(publicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return "", ErrInvalid
	}
	sum := sha256.Sum256(key)
	return hex.EncodeToString(sum[:]), nil
}
func ValidRoot(id string) bool {
	b, e := hex.DecodeString(id)
	return e == nil && len(b) == 32 && id == strings.ToLower(id)
}
func ValidID(id string) bool {
	if len(id) != 36 || id[8] != '-' || id[13] != '-' || id[18] != '-' || id[23] != '-' {
		return false
	}
	b, e := hex.DecodeString(strings.ReplaceAll(id, "-", ""))
	return e == nil && len(b) == 16 && id == strings.ToLower(id)
}
func Hash(body []byte) string { h := sha256.Sum256(body); return hex.EncodeToString(h[:]) }
func Proof(domain string, value any) []byte {
	b, _ := json.Marshal(value)
	// Sort object keys recursively and preserve integer spelling. Wire proofs
	// are independent of Go struct order and database JSON object ordering.
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	var canonical any
	if decoder.Decode(&canonical) == nil {
		b, _ = json.Marshal(canonical)
	}
	return append([]byte("pinelink-replication-"+domain+"-v1\n"), b...)
}
func Verify(publicKey string, proof []byte, signature string) bool {
	key, e := base64.StdEncoding.DecodeString(publicKey)
	if e != nil || len(key) != 32 {
		return false
	}
	sig, e := base64.StdEncoding.DecodeString(signature)
	return e == nil && ed25519.Verify(ed25519.PublicKey(key), proof, sig)
}
func Sign(key ed25519.PrivateKey, proof []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(key, proof))
}

// Routing is public; file names and recipient receipts remain in Ciphertext.
// Client packets describe history and never grant cloud-object access.
type Envelope struct {
	Version          int             `json:"version"`
	EventID          string          `json:"event_id"`
	SourceRoot       string          `json:"source_root"`
	StreamID         string          `json:"stream_id"`
	Sequence         int64           `json:"source_sequence"`
	AggregateID      string          `json:"aggregate_id"`
	AggregateVersion int64           `json:"aggregate_version"`
	Authority        string          `json:"authority"`
	AuthorityRoot    string          `json:"authority_root"`
	Kind             string          `json:"kind"`
	PeerRoot         string          `json:"peer_root,omitempty"`
	Recipients       []string        `json:"recipients"`
	SigningKey       string          `json:"signing_key"`
	Ciphertext       []byte          `json:"ciphertext,omitempty"`
	ServerPayload    json.RawMessage `json:"server_payload,omitempty"`
	Signature        string          `json:"signature"`
}

func (e Envelope) Proof() []byte { e.Signature = ""; return Proof("envelope", e) }
func (e Envelope) Validate() error {
	id, err := Root(e.SigningKey)
	if err != nil || id != e.SourceRoot || e.Version != Version || !ValidID(e.EventID) || !ValidID(e.StreamID) || e.Sequence < 1 || !ValidID(e.AggregateID) || e.AggregateVersion < 0 || !ValidRoot(e.AuthorityRoot) || len(e.Recipients) < 1 || len(e.Recipients) > 2 {
		return ErrInvalid
	}
	if !slices.IsSorted(e.Recipients) || len(slices.Compact(slices.Clone(e.Recipients))) != len(e.Recipients) {
		return ErrInvalid
	}
	for _, r := range e.Recipients {
		if !ValidRoot(r) {
			return ErrInvalid
		}
	}
	if e.Authority == "LOCAL_DEVICE" {
		if !ValidRoot(e.PeerRoot) || !slices.Contains(e.Recipients, e.SourceRoot) || len(e.Ciphertext) == 0 || len(e.Ciphertext) > MaxCiphertext || len(e.ServerPayload) != 0 {
			return ErrInvalid
		}
		for _, r := range e.Recipients {
			if r != e.SourceRoot && r != e.PeerRoot {
				return ErrInvalid
			}
		}
		switch e.Kind {
		case "TRANSFER_CHANGED":
			if e.AuthorityRoot != e.SourceRoot {
				return ErrInvalid
			}
		case "SENT_CHANGED":
			if e.AuthorityRoot != e.PeerRoot {
				return ErrInvalid
			}
		default:
			return ErrInvalid
		}
	} else if e.Authority == "SERVER" {
		if !strings.HasPrefix(e.Kind, "SERVER_") || len(e.Kind) > 128 || strings.ContainsAny(e.Kind, "\n\r| ") || e.AuthorityRoot != e.SourceRoot || len(e.Ciphertext) != 0 || len(e.ServerPayload) == 0 || len(e.ServerPayload) > MaxCiphertext {
			return ErrInvalid
		}
	} else {
		return ErrInvalid
	}
	if !Verify(e.SigningKey, e.Proof(), e.Signature) {
		return ErrInvalid
	}
	return nil
}

type Upload struct {
	SourceRoot string     `json:"source_root"`
	StreamID   string     `json:"stream_id"`
	After      int64      `json:"after"`
	Through    int64      `json:"through"`
	Events     []Envelope `json:"events"`
}

func (b Upload) Validate() error {
	if !ValidRoot(b.SourceRoot) || !ValidID(b.StreamID) || b.After < 0 || b.Through < b.After || len(b.Events) > MaxBatch {
		return ErrInvalid
	}
	last := b.After
	for _, e := range b.Events {
		if e.Validate() != nil || e.SourceRoot != b.SourceRoot || e.StreamID != b.StreamID || e.Authority != "LOCAL_DEVICE" || e.Sequence <= last || e.Sequence > b.Through {
			return ErrInvalid
		}
		last = e.Sequence
	}
	return nil
}

type Ack struct {
	Root      string `json:"source_root"`
	Stream    string `json:"stream_id"`
	Through   int64  `json:"through"`
	Epoch     string `json:"epoch"`
	Signature string `json:"signature"`
}

func (a Ack) Proof() []byte { a.Signature = ""; return Proof("ack", a) }

type Delivery struct {
	Sequence int64    `json:"sequence"`
	Event    Envelope `json:"event"`
}
type Page struct {
	Root      string     `json:"consumer_root"`
	Epoch     string     `json:"epoch"`
	After     int64      `json:"after"`
	Through   int64      `json:"through"`
	Upper     int64      `json:"upper"`
	Items     []Delivery `json:"items"`
	More      bool       `json:"more"`
	Snapshot  bool       `json:"snapshot,omitempty"`
	NextKey   string     `json:"next_key,omitempty"`
	Signature string     `json:"signature"`
}

func (p Page) Proof() []byte { p.Signature = ""; return Proof("page", p) }
func (p Page) Validate(key, root string) error {
	if p.Root != root || !ValidID(p.Epoch) || p.After < 0 || p.Through < p.After || p.Upper < p.Through || len(p.Items) > MaxBatch || !Verify(key, p.Proof(), p.Signature) {
		return ErrInvalid
	}
	last := p.After
	for _, d := range p.Items {
		if d.Event.Validate() != nil || !slices.Contains(d.Event.Recipients, root) || d.Sequence < 1 || d.Sequence > p.Upper {
			return ErrInvalid
		}
		if !p.Snapshot && (d.Sequence <= last || d.Sequence > p.Through) {
			return ErrInvalid
		}
		last = d.Sequence
	}
	if p.More && p.Through == p.After && !p.Snapshot {
		return ErrInvalid
	}
	return nil
}
func SnapshotKey(e Envelope) string {
	return e.SourceRoot + "|" + e.StreamID + "|" + e.Kind + "|" + e.AggregateID
}

type Info struct {
	Version   int    `json:"version"`
	Epoch     string `json:"epoch"`
	PublicKey string `json:"public_key"`
	UnixTime  int64  `json:"unix_time"`
	Nonce     string `json:"nonce"`
	Signature string `json:"signature"`
}

func (i Info) Proof() []byte { i.Signature = ""; return Proof("info", i) }
func (i Info) Validate(pin, nonce string) error {
	if i.Version != Version || !ValidID(i.Epoch) || i.PublicKey != pin || i.Nonce != nonce || i.UnixTime <= 0 || !Verify(pin, i.Proof(), i.Signature) {
		return ErrInvalid
	}
	return nil
}

type Binding struct {
	Root          string `json:"root"`
	SigningKey    string `json:"signing_key"`
	EncryptionKey string `json:"encryption_key"`
}
type Peer struct {
	Root          string `json:"root"`
	SigningKey    string `json:"signing_key"`
	EncryptionKey string `json:"encryption_key"`
}
type Roster struct {
	OperationID     string `json:"operation_id"`
	ExpectedVersion int64  `json:"expected_version"`
	Peers           []Peer `json:"peers"`
}
type Profile struct {
	Root          string `json:"root"`
	Epoch         string `json:"epoch"`
	RosterVersion int64  `json:"roster_version"`
	Uploaded      int64  `json:"uploaded"`
	Signature     string `json:"signature"`
}

func (p Profile) Proof() []byte { p.Signature = ""; return Proof("profile", p) }
func (p Peer) Validate() error {
	id, e := Root(p.SigningKey)
	if e != nil || id != p.Root || !strings.HasPrefix(p.EncryptionKey, "age1") || len(p.EncryptionKey) > 128 {
		return fmt.Errorf("%w: peer key", ErrInvalid)
	}
	return nil
}
