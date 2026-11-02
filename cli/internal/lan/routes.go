package lan

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/simiooo/pinelink/cli/internal/secure"
)

func (s *Server) route(w http.ResponseWriter, r *http.Request, actor string, body []byte) {
	if strings.HasPrefix(r.URL.Path, "/local/v1/replication/") {
		s.replicationRoute(w, r, actor, body)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/local/v1/")
	parts := strings.Split(path, "/")
	if path == "transfers" {
		if r.Method == "POST" {
			s.createTransfer(w, actor, body)
			return
		}
		if r.Method == "GET" {
			cursor := r.URL.Query().Get("cursor")
			if cursor != "" && !validTransferID(cursor) {
				localError(w, ErrConflict)
				return
			}
			s.mu.Lock()
			records := make([]*record, 0, len(s.transfers))
			for _, v := range s.transfers {
				records = append(records, v)
			}
			s.mu.Unlock()
			sort.Slice(records, func(i, j int) bool { return records[i].id < records[j].id })
			list := []Transfer{}
			for _, v := range records {
				v.mu.Lock()
				if v.t.ID > cursor && (actor == s.self.ID || v.t.PeerID == actor) {
					if err := s.expireLocked(v); err != nil {
						v.mu.Unlock()
						localError(w, err)
						return
					}
					list = append(list, v.snapshot())
				}
				v.mu.Unlock()
				if len(list) == 200 {
					break
				}
			}
			next := ""
			if len(list) == 200 {
				next = list[len(list)-1].ID
			}
			localJSON(w, 200, transferPage{Items: list, Next: next})
			return
		}
	}
	if len(parts) >= 2 && parts[0] == "transfers" {
		rec, err := s.get(parts[1])
		if err != nil {
			localError(w, err)
			return
		}
		rec.mu.Lock()
		allowed := actor == s.self.ID || rec.t.PeerID == actor
		rec.mu.Unlock()
		if !allowed {
			localError(w, ErrForbidden)
			return
		}
		if len(parts) == 2 && r.Method == "GET" {
			rec.mu.Lock()
			if err := s.expireLocked(rec); err != nil {
				rec.mu.Unlock()
				localError(w, err)
				return
			}
			t := rec.snapshot()
			rec.mu.Unlock()
			localJSON(w, 200, t)
			return
		}
		if len(parts) == 3 && r.Method == "POST" {
			if parts[2] == "complete" {
				s.complete(w, rec, actor)
				return
			}
			var input struct {
				Version int64 `json:"expected_version"`
			}
			if err = json.Unmarshal(body, &input); err != nil {
				localError(w, err)
				return
			}
			s.action(w, rec, actor, parts[2], input.Version)
			return
		}
		if len(parts) == 4 && parts[2] == "chunks" && r.Method == "PUT" {
			index, e := strconv.Atoi(parts[3])
			if e != nil {
				localError(w, e)
				return
			}
			s.chunk(w, rec, actor, index, r.Header.Get("X-Chunk-SHA256"), body)
			return
		}
	}
	if actor != s.self.ID {
		localError(w, ErrForbidden)
		return
	}
	if path == "invites" && r.Method == "POST" {
		i, err := s.Invite()
		if err != nil {
			localError(w, err)
		} else {
			localJSON(w, 200, i)
		}
		return
	}
	if path == "pair-requests" && r.Method == "GET" {
		s.mu.Lock()
		s.prune()
		pairs := []PairRequest{}
		for _, p := range s.pairs {
			if p.request.State == "WAITING" {
				pairs = append(pairs, p.request)
			}
		}
		s.mu.Unlock()
		localJSON(w, 200, pairs)
		return
	}
	if len(parts) == 3 && parts[0] == "pair-requests" && r.Method == "POST" {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.prune()
		p := s.pairs[parts[1]]
		if p == nil {
			localError(w, os.ErrNotExist)
			return
		}
		desired := map[string]string{"approve": "APPROVED", "reject": "REJECTED"}[parts[2]]
		if desired == "" {
			localError(w, ErrConflict)
			return
		}
		if p.request.State == desired {
			localJSON(w, 200, p.request)
			return
		}
		if p.request.State != "WAITING" {
			localError(w, ErrConflict)
			return
		}
		if desired == "APPROVED" {
			peer := p.request.Peer
			peer.Revoked = false
			previous, had := s.peers[peer.ID]
			s.peers[peer.ID] = peer
			if err := s.savePeer(peer); err != nil {
				if had {
					s.peers[peer.ID] = previous
				} else {
					delete(s.peers, peer.ID)
				}
				localError(w, err)
				return
			}
		}
		p.request.State = desired
		localJSON(w, 200, p.request)
		return
	}
	if path == "peers" && r.Method == "GET" {
		s.mu.Lock()
		peers := []Peer{}
		for _, p := range s.peers {
			peers = append(peers, p)
		}
		s.mu.Unlock()
		localJSON(w, 200, peers)
		return
	}
	if len(parts) == 3 && parts[0] == "peers" && parts[2] == "forget" && r.Method == "POST" {
		s.forget(w, parts[1])
		return
	}
	localError(w, os.ErrNotExist)
}

func (s *Server) commit(r *record, next Transfer) error {
	if err := writeJSON(s.root, recordName(next.ID), next); err != nil {
		return err
	}
	r.t = next
	r.active.Store(!next.terminal())
	return nil
}

func (s *Server) expireLocked(r *record) error {
	if !r.t.terminal() && !time.Now().Before(r.t.ExpiresAt) {
		next := r.t
		next.State = "EXPIRED"
		next.Version++
		if err := s.commit(r, next); err != nil {
			return err
		}
		if !r.working {
			_ = s.root.Remove(partName(next.ID))
		}
	}
	return nil
}

func (s *Server) createTransfer(w http.ResponseWriter, actor string, body []byte) {
	if actor == s.self.ID {
		localError(w, ErrForbidden)
		return
	}
	var offer Offer
	if err := json.Unmarshal(body, &offer); err != nil {
		localError(w, err)
		return
	}
	if !validTransferID(offer.ID) || offer.Size < 0 || offer.Size > MaxSize+(MaxSize/ChunkSize+1)*32 || len(offer.CipherRoot) != 64 || len(offer.Envelope) == 0 || len(offer.Envelope) > 64<<10 {
		localError(w, fmt.Errorf("invalid local file offer"))
		return
	}
	if raw, e := hex.DecodeString(offer.CipherRoot); e != nil || len(raw) != 32 {
		localError(w, fmt.Errorf("invalid ciphertext root"))
		return
	}
	manifest, err := secure.OpenManifest(offer.Envelope, s.identity)
	if err != nil {
		localError(w, fmt.Errorf("invalid encrypted manifest"))
		return
	}
	if manifest.PlaintextSize < 0 || manifest.PlaintextSize > MaxSize || len(manifest.PlaintextRoot) != 64 || manifest.FileName == "" || len(manifest.FileName) > 512 {
		localError(w, fmt.Errorf("invalid file metadata"))
		return
	}
	s.mu.Lock()
	if old := s.transfers[offer.ID]; old != nil {
		s.mu.Unlock()
		old.mu.Lock()
		defer old.mu.Unlock()
		if old.t.PeerID != actor || old.t.CipherRoot != offer.CipherRoot || old.t.Size != offer.Size || !bytes.Equal(old.t.Envelope, offer.Envelope) {
			localError(w, ErrConflict)
			return
		}
		if err := s.expireLocked(old); err != nil {
			localError(w, err)
			return
		}
		localJSON(w, 200, old.snapshot())
		return
	}
	active := 0
	for _, v := range s.transfers {
		if v.peerID == actor && v.active.Load() {
			active++
		}
	}
	if active >= 8 || len(s.transfers) >= 10_000 {
		s.mu.Unlock()
		localError(w, ErrConflict)
		return
	}
	state := "PENDING"
	if s.cfg.AutoAccept {
		state = "ACCEPTED"
	}
	next := Transfer{Offer: offer, PeerID: actor, FileName: manifest.FileName, PlainSize: manifest.PlaintextSize, PlainRoot: manifest.PlaintextRoot, State: state, Version: 1, ExpiresAt: time.Now().Add(s.cfg.TransferTTL), Route: "DIRECT", Transport: "LOCAL_HTTPS", SpoolState: "NOT_REQUIRED", Direction: "RECEIVE"}
	next.Authority, next.AuthorityID = "LOCAL_DEVICE", s.self.ID
	rec := newRecord(next)
	if err = writeJSON(s.root, recordName(offer.ID), next); err != nil {
		s.mu.Unlock()
		localError(w, err)
		return
	}
	s.transfers[offer.ID] = rec
	s.mu.Unlock()
	rec.mu.Lock()
	snapshot := rec.snapshot()
	rec.mu.Unlock()
	localJSON(w, 201, snapshot)
}

func (s *Server) action(w http.ResponseWriter, rec *record, actor, action string, version int64) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if err := s.expireLocked(rec); err != nil {
		localError(w, err)
		return
	}
	desired := map[string]string{"accept": "ACCEPTED", "reject": "REJECTED", "cancel": "CANCELLED", "pause": "PAUSED", "resume": "ACCEPTED"}[action]
	if desired == "" {
		localError(w, ErrConflict)
		return
	}
	if (action == "accept" || action == "reject") && actor != s.self.ID {
		localError(w, ErrForbidden)
		return
	}
	if rec.t.State == desired {
		localJSON(w, 200, rec.snapshot())
		return
	}
	if rec.t.Version != version || rec.t.terminal() {
		localError(w, ErrConflict)
		return
	}
	allowed := ((action == "accept" || action == "reject") && rec.t.State == "PENDING") || (action == "cancel") || (action == "pause" && rec.t.State == "ACCEPTED") || (action == "resume" && rec.t.State == "PAUSED")
	if !allowed {
		localError(w, ErrConflict)
		return
	}
	next := rec.t
	next.State = desired
	next.Version++
	if err := s.commit(rec, next); err != nil {
		localError(w, err)
		return
	}
	if next.terminal() && !rec.working {
		_ = s.root.Remove(partName(next.ID))
	}
	localJSON(w, 200, rec.snapshot())
}

func (s *Server) chunk(w http.ResponseWriter, rec *record, actor string, index int, hash string, body []byte) {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if err := s.expireLocked(rec); err != nil {
		localError(w, err)
		return
	}
	if actor != rec.t.PeerID || !s.trusted(actor) {
		localError(w, ErrForbidden)
		return
	}
	if rec.t.State != "ACCEPTED" {
		localError(w, ErrConflict)
		return
	}
	if index < 0 || int64(index) > rec.t.Size/ChunkSize {
		localError(w, ErrConflict)
		return
	}
	offset := int64(index) * ChunkSize
	expected := min(int64(ChunkSize), rec.t.Size-offset)
	if index < 0 || expected <= 0 || int64(len(body)) != expected || offset > rec.t.Acknowledged {
		localError(w, fmt.Errorf("invalid chunk position or length"))
		return
	}
	sum := sha256.Sum256(body)
	if fmt.Sprintf("%x", sum) != hash {
		localError(w, fmt.Errorf("chunk checksum mismatch"))
		return
	}
	file, err := s.root.OpenFile(partName(rec.t.ID), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		localError(w, err)
		return
	}
	defer file.Close()
	if offset < rec.t.Acknowledged {
		existing := make([]byte, len(body))
		if _, err = file.ReadAt(existing, offset); err != nil || !bytes.Equal(existing, body) {
			localError(w, ErrConflict)
			return
		}
		localJSON(w, 200, rec.snapshot())
		return
	}
	if err = file.Truncate(rec.t.Acknowledged); err == nil {
		_, err = file.WriteAt(body, offset)
	}
	if err == nil {
		err = file.Sync()
	}
	if err != nil {
		localError(w, err)
		return
	}
	if !s.trusted(actor) {
		_ = file.Truncate(rec.t.Acknowledged)
		localError(w, ErrForbidden)
		return
	}
	next := rec.t
	next.Acknowledged += int64(len(body))
	next.Revision++
	now := time.Now()
	next.SampledAt = &now
	if err = s.commit(rec, next); err != nil {
		_ = file.Truncate(rec.t.Acknowledged)
		localError(w, err)
		return
	}
	localJSON(w, 200, rec.snapshot())
}

func (s *Server) forget(w http.ResponseWriter, id string) {
	s.mu.Lock()
	peer, ok := s.peerLocked(id)
	if !ok {
		s.mu.Unlock()
		localError(w, os.ErrNotExist)
		return
	}
	peer.Revoked = true
	s.peers[id] = peer
	if err := s.savePeer(peer); err != nil {
		peer.Revoked = false
		s.peers[id] = peer
		s.mu.Unlock()
		localError(w, err)
		return
	}
	for token, session := range s.sessions {
		if session.peerID == id {
			delete(s.sessions, token)
		}
	}
	records := []*record{}
	for _, r := range s.transfers {
		records = append(records, r)
	}
	s.mu.Unlock()
	for _, r := range records {
		r.mu.Lock()
		if r.t.PeerID == id && !r.t.terminal() {
			next := r.t
			next.State = "CANCELLED"
			next.Version++
			next.Error = "PEER_FORGOTTEN"
			if err := s.commit(r, next); err != nil {
				r.mu.Unlock()
				localError(w, err)
				return
			}
		}
		r.mu.Unlock()
	}
	localJSON(w, 200, peer)
}

// Guarded reads keep a long verification responsive to cancellation/revocation.
type guardedReader struct {
	reader io.Reader
	check  func() error
}

func (r guardedReader) Read(p []byte) (int, error) {
	if err := r.check(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}
