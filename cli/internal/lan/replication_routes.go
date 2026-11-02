package lan

import (
	"encoding/json"
	"net/http"
	"strconv"

	rp "github.com/simiooo/pinelink/protocol/replication"
)

func (s *Server) replicationRoute(w http.ResponseWriter, r *http.Request, actor string, body []byte) {
	if !s.trusted(actor) && actor != s.self.ID {
		localError(w, ErrForbidden)
		return
	}
	path := r.URL.Path
	if r.Method == "GET" && path == "/local/v1/replication/info" {
		info, err := s.root.replicaInfo(s.identity, r.URL.Query().Get("nonce"))
		if err != nil {
			localError(w, err)
		} else {
			localJSON(w, 200, info)
		}
		return
	}
	if r.Method == "GET" && (path == "/local/v1/replication/events" || path == "/local/v1/replication/snapshot") {
		q := r.URL.Query()
		after, err := strconv.ParseInt(defaultLocal(q.Get("after"), "0"), 10, 64)
		if err != nil {
			localError(w, ErrConflict)
			return
		}
		upper, err := strconv.ParseInt(defaultLocal(q.Get("upper"), "-1"), 10, 64)
		if err != nil {
			localError(w, ErrConflict)
			return
		}
		page, err := s.root.replicaPage(s.identity, actor, q.Get("epoch"), after, path == "/local/v1/replication/snapshot", upper, q.Get("key"))
		if err != nil {
			localError(w, err)
		} else {
			localJSON(w, 200, page)
		}
		return
	}
	if r.Method == "POST" && path == "/local/v1/replication/events" {
		var batch rp.Upload
		if json.Unmarshal(body, &batch) != nil || batch.Validate() != nil || batch.SourceRoot != actor {
			localError(w, ErrForbidden)
			return
		}
		for _, e := range batch.Events {
			if e.PeerRoot != s.self.ID || !containsRecipient(e.Recipients, s.self.ID) {
				localError(w, ErrForbidden)
				return
			}
		}
		page := rp.Page{Root: s.self.ID, Epoch: s.root.streamID, After: batch.After, Through: batch.Through, Upper: batch.Through, Items: []rp.Delivery{}}
		for _, e := range batch.Events {
			page.Items = append(page.Items, rp.Delivery{Sequence: e.Sequence, Event: e})
		}
		page.Signature = s.identity.Sign(page.Proof())
		if _, err := s.root.applyReplicaPage(r.Context(), s.identity, "peer:"+actor, page, s.identity.SigningPublicKey(), batch); err != nil {
			localError(w, err)
			return
		}
		var cursor int64
		if err := s.root.db.QueryRow("SELECT sequence FROM replica_peer_uploads WHERE source_root=? AND stream_id=?", actor, batch.StreamID).Scan(&cursor); err != nil {
			localError(w, err)
			return
		}
		ack := rp.Ack{Root: actor, Stream: batch.StreamID, Through: cursor, Epoch: s.root.streamID}
		ack.Signature = s.identity.Sign(ack.Proof())
		localJSON(w, 200, ack)
		return
	}
	localError(w, ErrConflict)
}
func defaultLocal(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}
func containsRecipient(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
