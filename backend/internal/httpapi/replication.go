package httpapi

import (
	"net/http"
	"strconv"

	"github.com/simiooo/pinelink/backend/internal/auth"
	"github.com/simiooo/pinelink/backend/internal/service"
	rp "github.com/simiooo/pinelink/protocol/replication"
)

func (a *API) registerReplication(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/replication/info", func(w http.ResponseWriter, r *http.Request) {
		v, e := a.service.ReplicationInfo(r.Context(), r.URL.Query().Get("nonce"))
		respond(w, v, e)
	})
	mux.HandleFunc("POST /v1/replication/bind", func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.FromContext(r.Context())
		if !ok {
			writeError(w, service.ErrUnauthorized)
			return
		}
		var b rp.Binding
		if !readRequest(w, r, &b) {
			return
		}
		v, e := a.service.BindReplication(r.Context(), id.OwnerID, id.DeviceID, b)
		respond(w, v, e)
	})
	mux.HandleFunc("GET /v1/replication/profile", func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.FromContext(r.Context())
		if !ok {
			writeError(w, service.ErrUnauthorized)
			return
		}
		v, e := a.service.ReplicationProfile(r.Context(), id.OwnerID, id.DeviceID, r.URL.Query().Get("stream"))
		respond(w, v, e)
	})
	mux.HandleFunc("PUT /v1/replication/peers", func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.FromContext(r.Context())
		if !ok {
			writeError(w, service.ErrUnauthorized)
			return
		}
		var b rp.Roster
		if !readRequest(w, r, &b) {
			return
		}
		v, e := a.service.UpdateReplicationRoster(r.Context(), id.OwnerID, id.DeviceID, b)
		respond(w, v, e)
	})
	mux.HandleFunc("POST /v1/replication/events", func(w http.ResponseWriter, r *http.Request) {
		id, ok := auth.FromContext(r.Context())
		if !ok {
			writeError(w, service.ErrUnauthorized)
			return
		}
		var b rp.Upload
		if !readRequest(w, r, &b) {
			return
		}
		v, e := a.service.UploadReplication(r.Context(), id.OwnerID, id.DeviceID, b)
		respond(w, v, e)
	})
	read := func(snapshot bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			id, ok := auth.FromContext(r.Context())
			if !ok {
				writeError(w, service.ErrUnauthorized)
				return
			}
			q := r.URL.Query()
			after, err := strconv.ParseInt(defaultText(q.Get("after"), "0"), 10, 64)
			if err != nil {
				writeError(w, service.ErrInvalid)
				return
			}
			upper, err := strconv.ParseInt(defaultText(q.Get("upper"), "-1"), 10, 64)
			if err != nil {
				writeError(w, service.ErrInvalid)
				return
			}
			limit, err := strconv.Atoi(defaultText(q.Get("limit"), "100"))
			if err != nil {
				writeError(w, service.ErrInvalid)
				return
			}
			v, e := a.service.ReadReplication(r.Context(), id.OwnerID, id.DeviceID, q.Get("epoch"), after, limit, snapshot, upper, q.Get("key"))
			respond(w, v, e)
		}
	}
	mux.HandleFunc("GET /v1/replication/events", read(false))
	mux.HandleFunc("GET /v1/replication/snapshot", read(true))
}
func defaultText(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
