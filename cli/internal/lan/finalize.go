package lan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

func safeName(name, id string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\|?*`, r) {
			return '_'
		}
		return r
	}, name)
	name = strings.Trim(name, " .")
	if name == "" {
		name = "Received file"
	}
	reserved := strings.ToUpper(strings.SplitN(name, ".", 2)[0])
	if reserved == "CON" || reserved == "PRN" || reserved == "AUX" || reserved == "NUL" || (len(reserved) == 4 && (strings.HasPrefix(reserved, "COM") || strings.HasPrefix(reserved, "LPT"))) {
		name = "File " + name
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	trim := func(text string, limit int) string {
		for len(text) > limit {
			_, n := utf8.DecodeLastRuneInString(text)
			text = text[:len(text)-n]
		}
		return text
	}
	return trim(base, 170) + "-" + id + trim(ext, 32)
}

func (s *Server) complete(w http.ResponseWriter, rec *record, actor string) {
	rec.mu.Lock()
	if err := s.expireLocked(rec); err != nil {
		rec.mu.Unlock()
		localError(w, err)
		return
	}
	if rec.t.State == "COMPLETED" {
		snapshot := rec.snapshot()
		rec.mu.Unlock()
		localJSON(w, 200, snapshot)
		return
	}
	if actor != rec.t.PeerID || !s.trusted(actor) {
		rec.mu.Unlock()
		localError(w, ErrForbidden)
		return
	}
	if rec.t.State == "VERIFYING" {
		snapshot := rec.snapshot()
		rec.mu.Unlock()
		s.startFinalize(rec)
		localJSON(w, 202, snapshot)
		return
	}
	if rec.t.State != "ACCEPTED" || rec.t.Acknowledged != rec.t.Size {
		rec.mu.Unlock()
		localError(w, ErrConflict)
		return
	}
	next := rec.t
	next.State = "VERIFYING"
	next.Version++
	next.OutputName = safeName(next.FileName, next.ID)
	if err := s.commit(rec, next); err != nil {
		rec.mu.Unlock()
		localError(w, err)
		return
	}
	snapshot := rec.snapshot()
	rec.mu.Unlock()
	s.startFinalize(rec)
	localJSON(w, 202, snapshot)
}

func (s *Server) startFinalize(rec *record) {
	rec.mu.Lock()
	if rec.working || rec.t.State != "VERIFYING" {
		rec.mu.Unlock()
		return
	}
	if !s.beginWork() {
		rec.mu.Unlock()
		return
	}
	rec.working = true
	attempt := rec.t.Version
	rec.mu.Unlock()
	go func() {
		defer s.wg.Done()
		err := s.finalize(rec, attempt)
		rec.mu.Lock()
		defer rec.mu.Unlock()
		rec.working = false
		if err != nil && s.ctx.Err() == nil && rec.t.State == "VERIFYING" && rec.t.Version == attempt {
			next := rec.t
			next.State = "FAILED"
			next.Version++
			next.Error = "LOCAL_VERIFICATION_OR_COMMIT_FAILED"
			_ = s.commit(rec, next)
		}
		if rec.t.terminal() {
			_ = s.root.Remove(partName(rec.t.ID))
		}
	}()
}

func (s *Server) verifyGuard(rec *record, attempt int64) error {
	if err := s.ctx.Err(); err != nil {
		return err
	}
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if err := s.expireLocked(rec); err != nil {
		return err
	}
	if rec.t.State != "VERIFYING" || rec.t.Version != attempt || !s.trusted(rec.t.PeerID) {
		return ErrConflict
	}
	return nil
}

func hashFile(reader io.Reader) (string, int64, error) {
	h := sha256.New()
	n, err := io.CopyBuffer(h, reader, make([]byte, ChunkSize))
	return hex.EncodeToString(h.Sum(nil)), n, err
}

func (s *Server) finalize(rec *record, attempt int64) error {
	rec.mu.Lock()
	t := rec.t
	rec.mu.Unlock()
	guard := func() error { return s.verifyGuard(rec, attempt) }
	if err := guard(); err != nil {
		return err
	}
	cipher, err := s.root.OpenFile(partName(t.ID), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer cipher.Close()
	root, size, err := hashFile(guardedReader{reader: io.LimitReader(cipher, t.Size+1), check: guard})
	if err != nil {
		return err
	}
	if root != t.CipherRoot || size != t.Size {
		return fmt.Errorf("ciphertext integrity verification failed")
	}
	if _, err = cipher.Seek(0, io.SeekStart); err != nil {
		return err
	}
	manifest, err := secure.OpenManifest(t.Envelope, s.identity)
	if err != nil {
		return err
	}
	temp := ".pinelink-" + t.ID + ".plain"
	_ = s.receive.Remove(temp)
	output, err := s.receive.OpenFile(temp, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer s.receive.Remove(temp)
	plaintextHash := sha256.New()
	err = secure.DecryptManifestStream(guardedReader{reader: cipher, check: guard}, io.MultiWriter(output, plaintextHash), manifest)
	if err == nil {
		err = output.Sync()
	}
	info, statErr := output.Stat()
	closeErr := output.Close()
	if err != nil {
		return err
	}
	if statErr != nil {
		return statErr
	}
	if closeErr != nil {
		return closeErr
	}
	if info.Size() != manifest.PlaintextSize || hex.EncodeToString(plaintextHash.Sum(nil)) != manifest.PlaintextRoot {
		return fmt.Errorf("plaintext integrity verification failed")
	}
	if err = guard(); err != nil {
		return err
	}

	rec.mu.Lock()
	if rec.t.State != "VERIFYING" || rec.t.Version != attempt || !s.trusted(t.PeerID) {
		rec.mu.Unlock()
		return ErrConflict
	}
	linkErr := s.receive.Link(temp, t.OutputName)
	if linkErr != nil {
		rec.mu.Unlock()
		if !os.IsExist(linkErr) {
			return linkErr
		}
		info, err := s.receive.Lstat(t.OutputName)
		if err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("output already exists")
		}
		existing, err := s.receive.Open(t.OutputName)
		if err != nil {
			return err
		}
		stored, storedSize, err := hashFile(guardedReader{reader: existing, check: guard})
		_ = existing.Close()
		if err != nil {
			return err
		}
		if storedSize != manifest.PlaintextSize || stored != manifest.PlaintextRoot {
			return fmt.Errorf("output already exists with different content")
		}
		rec.mu.Lock()
	}
	defer rec.mu.Unlock()
	if err = s.ctx.Err(); err != nil {
		return err
	}
	if err := s.expireLocked(rec); err != nil {
		return err
	}
	if rec.t.State != "VERIFYING" || rec.t.Version != attempt || !s.trusted(t.PeerID) {
		return ErrConflict
	}
	if err = syncDirectory(s.receive); err != nil {
		return err
	}
	receipt := &Receipt{Version: Protocol, TransferID: t.ID, SenderID: t.PeerID, RecipientID: s.self.ID, CipherRoot: t.CipherRoot, PlainRoot: manifest.PlaintextRoot, PlainSize: manifest.PlaintextSize, DeliveredAt: time.Now().UTC()}
	receipt.Signature = s.identity.Sign(receipt.proof())
	next := rec.t
	next.State = "COMPLETED"
	next.Version++
	next.Receipt = receipt
	return s.commit(rec, next)
}
