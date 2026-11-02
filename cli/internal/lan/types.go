package lan

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"

	"filippo.io/age"
	"github.com/simiooo/pinelink/cli/internal/secure"
)

const Protocol = 1
const ChunkSize = 64 << 10
const MaxSize = int64(1) << 40
const ServiceType = "_pinelink._tcp"

var ErrConflict = errors.New("local transfer state conflict")
var ErrUnauthorized = errors.New("local peer authentication required")
var ErrForbidden = errors.New("local peer is not authorized")

type Peer struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	SigningKey    string `json:"signing_key"`
	EncryptionKey string `json:"encryption_key"`
	Address       string `json:"address"`
	Revoked       bool   `json:"revoked,omitempty"`
}

func Describe(identity *secure.Identity, name, address string) Peer {
	return Peer{ID: identity.LocalID(), Name: name, SigningKey: identity.SigningPublicKey(), EncryptionKey: identity.PublicKey(), Address: address}
}

func keyID(encoded string) (string, ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return "", nil, ErrUnauthorized
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), ed25519.PublicKey(raw), nil
}

func (p Peer) validate() error {
	id, _, err := keyID(p.SigningKey)
	if err != nil || id != p.ID {
		return ErrUnauthorized
	}
	if _, err = age.ParseX25519Recipient(p.EncryptionKey); err != nil {
		return err
	}
	if p.Name == "" || len(p.Name) > 128 || strings.TrimSpace(p.Name) != p.Name || strings.IndexFunc(p.Name, unicode.IsControl) >= 0 {
		return fmt.Errorf("invalid device name")
	}
	if p.Address != "" {
		if _, err := endpoint(p.Address); err != nil {
			return err
		}
	}
	return nil
}

func endpoint(address string) (*url.URL, error) {
	u, err := url.Parse(address)
	if err != nil {
		return nil, err
	}
	if u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Path != "" && u.Path != "/") {
		return nil, fmt.Errorf("invalid local endpoint")
	}
	if _, err = netip.ParseAddr(u.Hostname()); err != nil {
		return nil, fmt.Errorf("local endpoint must use an IP address")
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil || port < 1 || port > 65535 {
		return nil, fmt.Errorf("invalid local port")
	}
	u.Path = ""
	return u, nil
}

type Invitation struct {
	Version   int    `json:"version"`
	Peer      Peer   `json:"peer"`
	Token     string `json:"token"`
	ExpiresIn int    `json:"expires_in"`
	Signature string `json:"signature"`
}

func (i Invitation) proof() []byte {
	return []byte(fmt.Sprintf("pinelink-invite-v1\n%d\n%s\n%s\n%s\n%s\n%s\n%s\n%d", i.Version, i.Peer.ID, i.Peer.Name, i.Peer.SigningKey, i.Peer.EncryptionKey, i.Peer.Address, i.Token, i.ExpiresIn))
}
func (i Invitation) Code() (string, error) {
	b, err := json.Marshal(i)
	if err != nil {
		return "", err
	}
	return "pinelink://local/v1/" + base64.RawURLEncoding.EncodeToString(b), nil
}
func ParseInvitation(code string) (Invitation, error) {
	const prefix = "pinelink://local/v1/"
	var i Invitation
	if !strings.HasPrefix(code, prefix) || len(code) > 8192 {
		return i, fmt.Errorf("invalid connection code")
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimPrefix(code, prefix))
	if err != nil {
		return i, err
	}
	if err = json.Unmarshal(b, &i); err != nil {
		return i, err
	}
	if i.Version != Protocol || len(i.Token) != 43 || i.ExpiresIn < 1 || i.ExpiresIn > 120 {
		return i, fmt.Errorf("unsupported or invalid invitation")
	}
	if err = i.Peer.validate(); err != nil {
		return i, err
	}
	if _, err = endpoint(i.Peer.Address); err != nil {
		return i, err
	}
	_, pub, _ := keyID(i.Peer.SigningKey)
	sig, err := base64.StdEncoding.DecodeString(i.Signature)
	if err != nil || !ed25519.Verify(pub, i.proof(), sig) {
		return i, ErrUnauthorized
	}
	return i, nil
}

type PairRequest struct {
	ID    string `json:"id"`
	Peer  Peer   `json:"peer"`
	State string `json:"state"`
	Code  string `json:"code"`
}

func comparisonCode(token, target, source string) string {
	sum := sha256.Sum256([]byte("pinelink-compare-v1\n" + token + "\n" + target + "\n" + source))
	code := binary.BigEndian.Uint32(sum[:4]) % 1_000_000
	return fmt.Sprintf("%03d %03d", code/1000, code%1000)
}

type pairInput struct {
	Token     string `json:"token"`
	Peer      Peer   `json:"peer"`
	Signature string `json:"signature"`
}

func (p pairInput) proof(target string) []byte {
	return []byte("pinelink-pair-v1\n" + target + "\n" + p.Token + "\n" + p.Peer.ID + "\n" + p.Peer.Name + "\n" + p.Peer.SigningKey + "\n" + p.Peer.EncryptionKey + "\n" + p.Peer.Address)
}

type Offer struct {
	ID         string `json:"id"`
	CipherRoot string `json:"cipher_root"`
	Size       int64  `json:"size_bytes"`
	Envelope   []byte `json:"envelope"`
}

type Receipt struct {
	Version     int       `json:"version"`
	TransferID  string    `json:"transfer_id"`
	SenderID    string    `json:"sender_id"`
	RecipientID string    `json:"recipient_id"`
	CipherRoot  string    `json:"cipher_root"`
	PlainRoot   string    `json:"plaintext_root"`
	PlainSize   int64     `json:"plaintext_size"`
	DeliveredAt time.Time `json:"delivered_at"`
	Signature   string    `json:"signature"`
}

func (r Receipt) proof() []byte {
	return []byte(fmt.Sprintf("pinelink-receipt-v1\n%d\n%s\n%s\n%s\n%s\n%s\n%d\n%s", r.Version, r.TransferID, r.SenderID, r.RecipientID, r.CipherRoot, r.PlainRoot, r.PlainSize, r.DeliveredAt.UTC().Format(time.RFC3339Nano)))
}
func (r Receipt) Verify(peer Peer, offer Offer, sender string) error {
	if r.Version != Protocol || r.TransferID != offer.ID || r.CipherRoot != offer.CipherRoot || r.SenderID != sender || r.RecipientID != peer.ID {
		return ErrUnauthorized
	}
	_, key, err := keyID(peer.SigningKey)
	if err != nil {
		return err
	}
	sig, err := base64.StdEncoding.DecodeString(r.Signature)
	if err != nil || !ed25519.Verify(key, r.proof(), sig) {
		return ErrUnauthorized
	}
	return nil
}

type Transfer struct {
	SampleAgeMillis int64 `json:"sample_age_ms"`
	Offer
	PeerID          string     `json:"peer_id"`
	FileName        string     `json:"file_name"`
	PlainSize       int64      `json:"plaintext_size"`
	PlainRoot       string     `json:"plaintext_root"`
	State           string     `json:"state"`
	Version         int64      `json:"state_version"`
	Acknowledged    int64      `json:"acknowledged_bytes"`
	Revision        int64      `json:"progress_revision"`
	SampledAt       *time.Time `json:"sampled_at,omitempty"`
	ExpiresAt       time.Time  `json:"expires_at"`
	RemainingMillis int64      `json:"remaining_ms"`
	Route           string     `json:"route"`
	Transport       string     `json:"transport"`
	Authority       string     `json:"authority"`
	AuthorityID     string     `json:"authority_device_id"`
	SpoolState      string     `json:"spool_state"`
	Receipt         *Receipt   `json:"receipt,omitempty"`
	OutputName      string     `json:"output_name,omitempty"`
	Direction       string     `json:"direction"`
	Error           string     `json:"error,omitempty"`
}

func (t Transfer) terminal() bool {
	return t.State == "COMPLETED" || t.State == "REJECTED" || t.State == "CANCELLED" || t.State == "EXPIRED" || t.State == "FAILED"
}

type Config struct {
	Directory        string
	ReceiveDirectory string
	Name             string
	Address          string
	AutoPair         bool
	AutoAccept       bool
	TransferTTL      time.Duration
	Sync             bool
}

type transferPage struct {
	Items []Transfer `json:"items"`
	Next  string     `json:"next_cursor,omitempty"`
}
