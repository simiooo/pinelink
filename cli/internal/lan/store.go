package lan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

type record struct {
	mu      sync.Mutex
	t       Transfer
	working bool
	peerID  string
	id      string
	active  atomic.Bool
}

func newRecord(t Transfer) *record {
	r := &record{t: t, peerID: t.PeerID, id: t.ID}
	r.active.Store(!t.terminal())
	return r
}

func openDirectory(path string) (*os.Root, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(absolute, 0o700); err != nil {
		return nil, err
	}
	return os.OpenRoot(absolute)
}

func (r *record) snapshot() Transfer {
	t := r.t
	t.Envelope = nil
	if t.SampledAt != nil {
		t.SampleAgeMillis = max(0, time.Since(*t.SampledAt).Milliseconds())
	}
	t.RemainingMillis = max(0, time.Until(t.ExpiresAt).Milliseconds())
	return t
}

func validTransferID(id string) bool {
	parsed, err := uuid.Parse(id)
	return err == nil && parsed.String() == id
}
func recordName(id string) string      { return "transfer-" + id + ".json" }
func partName(id string) string        { return "transfer-" + id + ".cipher" }
func (s *Server) save(r *record) error { return writeJSON(s.root, recordName(r.t.ID), r.t) }

func (s *Server) loadRecords() error {
	entries, err := s.root.keys("peer", "transfer")
	if err != nil {
		return err
	}
	for _, name := range entries {
		if strings.HasPrefix(name, "peer-") && strings.HasSuffix(name, ".json") {
			var peer Peer
			if err = readJSON(s.root, name, &peer); err != nil {
				return err
			}
			if err = peer.validate(); err != nil {
				return err
			}
			if name != "peer-"+peer.ID+".json" {
				return ErrUnauthorized
			}
			s.peers[peer.ID] = peer
			continue
		}
		if !strings.HasPrefix(name, "transfer-") || !strings.HasSuffix(name, ".json") {
			continue
		}
		var t Transfer
		if err = readJSON(s.root, name, &t); err != nil {
			return err
		}
		if !validTransferID(t.ID) || recordName(t.ID) != name || t.Acknowledged < 0 || t.Acknowledged > t.Size {
			return fmt.Errorf("invalid local transfer record")
		}
		s.transfers[t.ID] = newRecord(t)
	}
	return nil
}

// Both directions share one per-peer trust decision. A foreground receiver
// observes pairing/revocation performed by another local CLI process.
func (s *Server) savePeer(peer Peer) error { return writeJSON(s.root, "peer-"+peer.ID+".json", peer) }
func (s *Server) peerLocked(id string) (Peer, bool) {
	if len(id) != 64 {
		return Peer{}, false
	}
	var peer Peer
	if err := readJSON(s.root, "peer-"+id+".json", &peer); err == nil && peer.ID == id && peer.validate() == nil {
		s.peers[id] = peer
		return peer, true
	}
	return Peer{}, false
}
