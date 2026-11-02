package lan

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

func (s *Server) prune() {
	now := time.Now()
	for k, v := range s.sessions {
		if !now.Before(v.deadline) {
			delete(s.sessions, k)
		}
	}
	for k, v := range s.challenges {
		if !now.Before(v.deadline) {
			delete(s.challenges, k)
		}
	}
	for k, v := range s.invites {
		if !now.Before(v.deadline) {
			delete(s.invites, k)
		}
	}
	for k, v := range s.pairs {
		if !now.Before(v.deadline) && v.request.State == "WAITING" {
			v.request.State = "EXPIRED"
		}
		if now.After(v.deadline.Add(10 * time.Minute)) {
			delete(s.pairs, k)
		}
	}
}

func verifyProof(key, proof string, message []byte) bool {
	_, pub, err := keyID(key)
	if err != nil {
		return false
	}
	sig, err := base64.StdEncoding.DecodeString(proof)
	return err == nil && ed25519.Verify(pub, message, sig)
}
func sessionProof(target, boot, nonce, peer string) []byte {
	return []byte("pinelink-session-v1\n" + target + "\n" + boot + "\n" + nonce + "\n" + peer)
}
func requestProof(session, nonce, method, uri string, body []byte) []byte {
	sum := sha256.Sum256(body)
	return []byte(fmt.Sprintf("pinelink-request-v1\n%s\n%s\n%s\n%s\n%x", session, nonce, method, uri, sum))
}

func (s *Server) handleChallenge(w http.ResponseWriter, r *http.Request) {
	peer := r.URL.Query().Get("peer_id")
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	p, ok := s.peerLocked(peer)
	if peer != s.self.ID && (!ok || p.Revoked) {
		localError(w, ErrForbidden)
		return
	}
	if len(s.challenges) >= 256 {
		localError(w, ErrConflict)
		return
	}
	nonce := randomToken()
	s.challenges[nonce] = challenge{peerID: peer, deadline: time.Now().Add(30 * time.Second)}
	localJSON(w, 200, map[string]any{"nonce": nonce, "boot": s.boot, "target_id": s.self.ID})
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PeerID    string `json:"peer_id"`
		Nonce     string `json:"nonce"`
		Signature string `json:"signature"`
	}
	if err := decodeRequest(w, r, &input); err != nil {
		localError(w, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	c, ok := s.challenges[input.Nonce]
	if !ok || c.peerID != input.PeerID {
		localError(w, ErrUnauthorized)
		return
	}
	delete(s.challenges, input.Nonce)
	peer, ok := s.peerLocked(input.PeerID)
	if input.PeerID == s.self.ID {
		peer, ok = s.self, true
	}
	if !ok || peer.Revoked || !verifyProof(peer.SigningKey, input.Signature, sessionProof(s.self.ID, s.boot, input.Nonce, input.PeerID)) {
		localError(w, ErrUnauthorized)
		return
	}
	count, oldestToken := 0, ""
	var oldest time.Time
	for token, existing := range s.sessions {
		if existing.peerID == input.PeerID {
			count++
			if oldestToken == "" || existing.deadline.Before(oldest) {
				oldestToken, oldest = token, existing.deadline
			}
		}
	}
	if count >= 16 {
		delete(s.sessions, oldestToken)
	}
	if len(s.sessions) >= 256 {
		localError(w, ErrConflict)
		return
	}
	token := randomToken()
	s.sessions[token] = &session{peerID: input.PeerID, deadline: time.Now().Add(10 * time.Minute), seen: map[string]bool{}}
	localJSON(w, 200, map[string]any{"token": token, "expires_in": 600, "peer": s.self})
}

func (s *Server) authenticate(r *http.Request, body []byte) (string, error) {
	token, nonce, signature := r.Header.Get("X-Pinelink-Local-Session"), r.Header.Get("X-Pinelink-Local-Nonce"), r.Header.Get("X-Pinelink-Local-Signature")
	if len(token) != 43 || len(nonce) != 43 {
		return "", ErrUnauthorized
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[token]
	if !ok || !time.Now().Before(session.deadline) {
		delete(s.sessions, token)
		return "", ErrUnauthorized
	}
	peer, ok := s.peerLocked(session.peerID)
	if session.peerID == s.self.ID {
		peer, ok = s.self, true
	}
	if !ok || peer.Revoked {
		return "", ErrForbidden
	}
	if !verifyProof(peer.SigningKey, signature, requestProof(token, nonce, r.Method, r.URL.RequestURI(), body)) {
		return "", ErrUnauthorized
	}
	if r.Method != "GET" {
		if session.seen[nonce] {
			return "", ErrConflict
		}
		if len(session.seen) >= 4096 {
			return "", ErrUnauthorized
		}
		session.seen[nonce] = true
	}
	return peer.ID, nil
}

func (s *Server) handlePair(w http.ResponseWriter, r *http.Request) {
	var input pairInput
	if err := decodeRequest(w, r, &input); err != nil {
		localError(w, err)
		return
	}
	if err := input.Peer.validate(); err != nil {
		localError(w, err)
		return
	}
	input.Peer.Revoked = false
	if !verifyProof(input.Peer.SigningKey, input.Signature, input.proof(s.self.ID)) || input.Peer.ID == s.self.ID {
		localError(w, ErrUnauthorized)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	invite := s.invites[input.Token]
	if invite == nil {
		localError(w, ErrUnauthorized)
		return
	}
	if invite.pairID != "" {
		pair := s.pairs[invite.pairID]
		if pair == nil || pair.request.Peer.ID != input.Peer.ID || pair.request.Peer.EncryptionKey != input.Peer.EncryptionKey {
			localError(w, ErrForbidden)
			return
		}
		localJSON(w, 200, pair.request)
		return
	}
	if len(s.pairs) >= 32 || len(s.peers) >= 1024 {
		localError(w, ErrConflict)
		return
	}
	pair := &pendingPair{request: PairRequest{ID: uuid.NewString(), Peer: input.Peer, State: "WAITING", Code: comparisonCode(input.Token, s.self.ID, input.Peer.ID)}, deadline: invite.deadline, token: input.Token}
	s.pairs[pair.request.ID] = pair
	invite.pairID = pair.request.ID
	if s.cfg.AutoPair {
		s.peers[input.Peer.ID] = input.Peer
		if err := s.savePeer(input.Peer); err != nil {
			delete(s.peers, input.Peer.ID)
			localError(w, err)
			return
		}
		pair.request.State = "APPROVED"
	}
	localJSON(w, 200, pair.request)
}

func (s *Server) handlePairStatus(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID        string `json:"id"`
		Signature string `json:"signature"`
	}
	if err := decodeRequest(w, r, &input); err != nil {
		localError(w, err)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prune()
	pair := s.pairs[input.ID]
	if pair == nil || !verifyProof(pair.request.Peer.SigningKey, input.Signature, []byte("pinelink-pair-status-v1\n"+s.self.ID+"\n"+input.ID)) {
		localError(w, ErrUnauthorized)
		return
	}
	localJSON(w, 200, pair.request)
}
