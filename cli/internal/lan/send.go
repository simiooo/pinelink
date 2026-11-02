package lan

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/simiooo/pinelink/cli/internal/secure"
)

type sentRecord struct {
	AutoSend    bool     `json:"auto_send,omitempty"`
	Offer       Offer    `json:"offer"`
	Peer        Peer     `json:"peer"`
	CipherName  string   `json:"cipher_name"`
	OwnEnvelope []byte   `json:"own_envelope"`
	PlainRoot   string   `json:"plaintext_root"`
	PlainSize   int64    `json:"plaintext_size"`
	Status      Transfer `json:"status"`
}

func (c *Client) AutomaticSend(id string, enabled bool) error {
	var sent sentRecord
	if !validTransferID(id) {
		return ErrConflict
	}
	if err := readJSON(c.root, sentName(id), &sent); err != nil {
		return err
	}
	sent.AutoSend = enabled
	return writeJSON(c.root, sentName(id), sent)
}

func sentName(id string) string { return "sent-" + id + ".json" }

func (c *Client) PrepareFile(ctx context.Context, input string) (Transfer, error) {
	info, err := os.Stat(input)
	if err != nil {
		return Transfer{}, err
	}
	if !info.Mode().IsRegular() || info.Size() > MaxSize {
		return Transfer{}, fmt.Errorf("input must be a regular file within the local transfer limit")
	}
	source, err := os.Open(input)
	if err != nil {
		return Transfer{}, err
	}
	defer source.Close()
	plainRoot, plainSize, err := hashFile(guardedReader{reader: io.LimitReader(source, MaxSize+1), check: ctx.Err})
	if err != nil {
		return Transfer{}, err
	}
	if plainSize > MaxSize {
		return Transfer{}, fmt.Errorf("file exceeds local transfer limit")
	}
	plain := struct {
		ContentRoot string
		Size        int64
	}{plainRoot, plainSize}
	if _, err = source.Seek(0, io.SeekStart); err != nil {
		return Transfer{}, err
	}
	id := uuid.NewString()
	cipherName := "sent-" + id + ".cipher"
	destination, err := c.root.OpenFile(cipherName, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		return Transfer{}, err
	}
	retained := false
	defer func() {
		destination.Close()
		if !retained {
			_ = c.root.Remove(cipherName)
		}
	}()
	plaintextHash := sha256.New()
	envelope, err := secure.EncryptManifestStream(guardedReader{reader: io.TeeReader(source, plaintextHash), check: ctx.Err}, destination, plain.ContentRoot, plain.Size, filepath.Base(input), c.identity)
	if err != nil {
		return Transfer{}, err
	}
	if hex.EncodeToString(plaintextHash.Sum(nil)) != plain.ContentRoot {
		return Transfer{}, fmt.Errorf("input changed while preparing the encrypted file")
	}
	if err = destination.Sync(); err != nil {
		return Transfer{}, err
	}
	if _, err = destination.Seek(0, io.SeekStart); err != nil {
		return Transfer{}, err
	}
	cipherRoot, cipherSize, err := hashFile(guardedReader{reader: destination, check: ctx.Err})
	if err != nil {
		return Transfer{}, err
	}
	manifest, err := secure.OpenManifest(envelope, c.identity)
	if err != nil {
		return Transfer{}, err
	}
	recipientEnvelope, err := secure.SealManifest(manifest, []string{c.peer.EncryptionKey})
	if err != nil {
		return Transfer{}, err
	}
	offer := Offer{ID: id, CipherRoot: cipherRoot, Size: cipherSize, Envelope: recipientEnvelope}
	saved := sentRecord{Offer: offer, Peer: c.peer, CipherName: cipherName, OwnEnvelope: envelope, PlainRoot: plain.ContentRoot, PlainSize: plain.Size,
		Status: Transfer{Offer: Offer{ID: id, CipherRoot: cipherRoot, Size: cipherSize}, PeerID: c.peer.ID, FileName: manifest.FileName, PlainSize: plain.Size, PlainRoot: plain.ContentRoot, State: "PREPARING", Direction: "SEND", Route: "DIRECT", Transport: "LOCAL_HTTPS", SpoolState: "NOT_REQUIRED"}}
	saved.Status.Authority, saved.Status.AuthorityID = "LOCAL_DEVICE", c.peer.ID
	if err = writeJSON(c.root, sentName(id), saved); err != nil {
		return Transfer{}, err
	}
	retained = true
	// Persist the immutable attempt before a request that could succeed while
	// its response is lost. Retrying this UUID cannot create a second delivery.
	body, _ := json.Marshal(offer)
	var t Transfer
	if err = c.request(ctx, "POST", "transfers", body, &t); err != nil {
		return saved.Status, err
	}
	saved.Status = sendSnapshot(t, c.peer.ID)
	if err = writeJSON(c.root, sentName(id), saved); err != nil {
		return Transfer{}, err
	}
	return saved.Status, nil
}

func sendSnapshot(t Transfer, peerID string) Transfer {
	if t.Direction == "SEND" {
		t.PeerID = peerID
		t.Envelope = nil
		return t
	}
	t.Direction = "SEND"
	if t.SampledAt != nil {
		localSample := time.Now().Add(-time.Duration(t.SampleAgeMillis) * time.Millisecond)
		t.SampledAt = &localSample
	}
	t.ExpiresAt = time.Now().Add(time.Duration(t.RemainingMillis) * time.Millisecond)
	t.PeerID = peerID
	t.Envelope = nil
	return t
}
func waitLocal(ctx context.Context) error {
	timer := time.NewTimer(150 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func (c *Client) Send(ctx context.Context, id string, observe func(Transfer)) (result Transfer, resultErr error) {
	if !validTransferID(id) {
		return result, fmt.Errorf("invalid transfer id")
	}
	lock, err := c.root.OpenFile("sent-"+id+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return result, err
	}
	defer lock.Close()
	for {
		err = lockDirectory(lock)
		if err == nil {
			break
		}
		if !lockBusy(err) {
			return result, err
		}
		var current sentRecord
		if err = readJSON(c.root, sentName(id), &current); err != nil {
			return result, err
		}
		if current.Offer.ID != id || current.Peer.ID != c.peer.ID || current.Peer.SigningKey != c.peer.SigningKey {
			return result, ErrForbidden
		}
		result = current.Status
		if observe != nil {
			observe(result)
		}
		if err = waitLocal(ctx); err != nil {
			return result, err
		}
	}
	var saved sentRecord
	if err = readJSON(c.root, sentName(id), &saved); err != nil {
		return result, err
	}
	if saved.Offer.ID != id || saved.Peer.ID != c.peer.ID || saved.Peer.SigningKey != c.peer.SigningKey {
		return result, ErrForbidden
	}
	defer func() {
		if result.ID != "" {
			saved.Status = sendSnapshot(result, c.peer.ID)
			if writeJSON(c.root, sentName(id), saved) == nil && result.terminal() {
				_ = c.root.Remove(saved.CipherName)
			}
		}
	}()
	if saved.Status.State == "COMPLETED" && saved.Status.Receipt != nil {
		return c.confirmReceipt(saved, saved.Status)
	}
	if saved.Status.terminal() {
		return saved.Status, fmt.Errorf("local transfer is %s", saved.Status.State)
	}
	body, _ := json.Marshal(saved.Offer)
	if err = c.request(ctx, "POST", "transfers", body, &result); err != nil {
		return result, err
	}
	if result.State == "COMPLETED" {
		return c.confirmReceipt(saved, result)
	}
	if result.Version < saved.Status.Version || result.Acknowledged < saved.Status.Acknowledged {
		return result, fmt.Errorf("recipient checkpoint regressed")
	}
	if result.terminal() {
		return result, fmt.Errorf("local transfer is %s", result.State)
	}
	file, err := c.root.Open(saved.CipherName)
	if err != nil {
		return result, err
	}
	defer file.Close()
	lastSaved := time.Time{}
	knownVersion, knownAck := saved.Status.Version, saved.Status.Acknowledged
	for {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		if result.ID != id || result.CipherRoot != saved.Offer.CipherRoot || result.Size != saved.Offer.Size || result.Acknowledged < 0 || result.Acknowledged > result.Size || result.Authority != "LOCAL_DEVICE" || result.AuthorityID != c.peer.ID {
			return result, ErrUnauthorized
		}
		if result.Version < knownVersion || result.Acknowledged < knownAck {
			return result, fmt.Errorf("recipient checkpoint regressed")
		}
		knownVersion, knownAck = result.Version, result.Acknowledged
		if result.State == "COMPLETED" {
			return c.confirmReceipt(saved, result)
		}
		if result.terminal() {
			return result, fmt.Errorf("local transfer is %s", result.State)
		}
		if result.RemainingMillis <= 0 {
			return result, fmt.Errorf("local transfer has expired")
		}
		if observe != nil {
			observe(sendSnapshot(result, c.peer.ID))
		}
		if time.Since(lastSaved) > time.Second {
			saved.Status = sendSnapshot(result, c.peer.ID)
			if err = writeJSON(c.root, sentName(id), saved); err != nil {
				if errors.Is(err, ErrConflict) {
					result, err = c.Transfer(ctx, id)
					if err == nil {
						continue
					}
				}
				return result, err
			}
			lastSaved = time.Now()
		}
		switch result.State {
		case "PENDING", "PAUSED", "VERIFYING":
			if err = waitLocal(ctx); err != nil {
				return result, err
			}
			result, err = c.Transfer(ctx, id)
		case "ACCEPTED":
			if result.Acknowledged == result.Size {
				err = c.request(ctx, "POST", "transfers/"+id+"/complete", nil, &result)
				break
			}
			if result.Acknowledged%ChunkSize != 0 {
				return result, ErrConflict
			}
			length := min(int64(ChunkSize), result.Size-result.Acknowledged)
			data := make([]byte, int(length))
			if _, err = file.ReadAt(data, result.Acknowledged); err != nil {
				return result, err
			}
			path := "transfers/" + id + "/chunks/" + strconv.FormatInt(result.Acknowledged/ChunkSize, 10)
			for retry := 0; retry < 3; retry++ {
				err = c.request(ctx, "PUT", path, data, &result)
				if err == nil {
					break
				}
				var status *StatusError
				if errors.As(err, &status) {
					if status.Status == 409 {
						result, err = c.Transfer(ctx, id)
					}
					break
				}
				if waitErr := waitLocal(ctx); waitErr != nil {
					return result, waitErr
				}
			}
		default:
			return result, ErrConflict
		}
		if err != nil {
			return result, err
		}
	}
}

func (c *Client) confirmReceipt(saved sentRecord, t Transfer) (Transfer, error) {
	if t.Authority != "LOCAL_DEVICE" || t.AuthorityID != c.peer.ID {
		return t, ErrUnauthorized
	}
	if t.Receipt == nil {
		return t, fmt.Errorf("recipient completion has no signed receipt")
	}
	if err := t.Receipt.Verify(c.peer, saved.Offer, c.self.ID); err != nil {
		return t, err
	}
	if t.Receipt.PlainRoot != saved.PlainRoot || t.Receipt.PlainSize != saved.PlainSize {
		return t, fmt.Errorf("recipient receipt does not match the source file")
	}
	saved.Status = sendSnapshot(t, c.peer.ID)
	if err := writeJSON(c.root, sentName(t.ID), saved); err != nil {
		return t, err
	}
	_ = c.root.Remove(saved.CipherName)
	return saved.Status, nil
}

func SavedPeer(directory, id string) (Peer, error) {
	if !validTransferID(id) {
		return Peer{}, fmt.Errorf("invalid transfer id")
	}
	root, err := openStateDirectory(directory, "")
	if err != nil {
		return Peer{}, err
	}
	defer root.Close()
	var record sentRecord
	if err = readJSON(root, sentName(id), &record); err != nil {
		return Peer{}, err
	}
	if record.Status.State == "COMPLETED" {
		return record.Peer, nil
	}
	return ReadContact(directory, record.Peer.ID)
}
