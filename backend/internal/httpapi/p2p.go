package httpapi

import (
	"github.com/simiooo/pinelink/backend/internal/auth"
	"net/http"
	"strings"
	"time"

	"github.com/simiooo/pinelink/backend/internal/service"
)

func (a *API) registerP2P(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/p2p", a.createP2P)
	mux.HandleFunc("POST /v1/p2p/text", a.createP2P)
	mux.HandleFunc("POST /v1/p2p/{id}/signals", a.addP2PSignal)
	mux.HandleFunc("GET /v1/p2p/{id}/signals", a.p2pSignals)
	mux.HandleFunc("POST /v1/p2p/{id}/progress", a.p2pProgress)
	mux.HandleFunc("GET /v1/p2p/{id}/progress", a.p2pProgressSnapshot)
	mux.HandleFunc("POST /v1/p2p/{id}/complete", a.completeP2P)
	mux.HandleFunc("GET /v1/p2p/{id}/relay-transport", a.relayTransport)
	mux.HandleFunc("GET /v1/p2p/{id}/relay", a.relaySocket)
	mux.HandleFunc("POST /v1/p2p/{id}/relay-ticket", a.mintRelayTicket)
}

func (a *API) p2pProgressSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.service.GetP2PProgress(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"))
	respond(w, snapshot, err)
}

func (a *API) createP2P(w http.ResponseWriter, r *http.Request) {
	if i, ok := auth.FromContext(r.Context()); ok && i.GuestSessionID != "" {
		if err := a.service.Accounts().Rate(r.Context(), "guest-transfer/"+i.OwnerID, 120, time.Hour); err != nil {
			authFailure(w, err)
			return
		}
	}
	var req service.P2PCreateRequest
	if !readRequest(w, r, &req) {
		return
	}
	// A distinct endpoint fails closed on older servers which ignore unknown
	// request fields. It cannot silently create a legacy file transfer.
	if r.URL.Path == "/v1/p2p/text" {
		req.ContentKind = "TEXT"
	} else if req.ContentKind != "" {
		writeError(w, service.ErrInvalid)
		return
	}
	transfer, err := a.service.CreateP2P(r.Context(), req)
	respond(w, transfer, err)
}

func (a *API) addP2PSignal(w http.ResponseWriter, r *http.Request) {
	if !a.guestSignalRate(w, r, false) {
		return
	}
	var req service.P2PSignalRequest
	if !readRequest(w, r, &req) {
		return
	}
	if a.service.RelayTransport() == "websocket" && strings.HasPrefix(req.Channel, "relay-") {
		writeError(w, service.ErrInvalid)
		return
	}
	signal, err := a.service.AddSignal(r.Context(), r.PathValue("id"), req)
	respond(w, signal, err)
}

func (a *API) p2pSignals(w http.ResponseWriter, r *http.Request) {
	if !a.guestSignalRate(w, r, true) {
		return
	}
	signals, err := a.service.Signals(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"), parseInt64(r.URL.Query().Get("after")))
	respond(w, signals, err)
}

func (a *API) guestSignalRate(w http.ResponseWriter, r *http.Request, read bool) bool {
	i, ok := auth.FromContext(r.Context())
	if !ok || i.GuestSessionID == "" {
		return true
	}
	kind, limit := "write", 60
	if read {
		kind, limit = "read", 600
	}
	if err := a.service.Accounts().Rate(r.Context(), "guest-signal-"+kind+"/"+i.OwnerID, limit, time.Minute); err != nil {
		authFailure(w, err)
		return false
	}
	if err := a.service.Accounts().Rate(r.Context(), "guest-signal-ip-"+kind+"/"+i.ResourceIP, limit*4, time.Minute); err != nil {
		authFailure(w, err)
		return false
	}
	return true
}

func (a *API) p2pProgress(w http.ResponseWriter, r *http.Request) {
	var req service.P2PProgressRequest
	if !readRequest(w, r, &req) {
		return
	}
	if err := a.service.UpdateP2PProgress(r.Context(), r.PathValue("id"), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "recorded"})
}

func (a *API) completeP2P(w http.ResponseWriter, r *http.Request) {
	var req service.TransferActionRequest
	if !readRequest(w, r, &req) {
		return
	}
	transfer, err := a.service.CompleteP2P(r.Context(), r.PathValue("id"), req)
	if err != nil {
		respond(w, transfer, err)
		return
	}
	// Keep the established top-level transfer fields for native clients. A
	// browser can commit the receipt and its authoritative route/ledger together
	// without briefly rendering a completed file with an older polling snapshot.
	progress, projectionErr := a.service.GetP2PProgress(r.Context(), transfer.ID, req.OwnerID, req.DeviceID)
	if projectionErr != nil {
		respond(w, transfer, nil)
		return
	}
	respond(w, struct {
		service.Transfer
		Progress service.P2PProgressSnapshot `json:"progress"`
	}{transfer, progress}, nil)
}
