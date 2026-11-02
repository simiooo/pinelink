package httpapi

import (
	"net/http"
	"strings"

	"github.com/simiooo/pinelink/backend/internal/service"
)

func (a *API) registerP2P(mux *http.ServeMux) {
	mux.HandleFunc("POST /v1/p2p", a.createP2P)
	mux.HandleFunc("POST /v1/p2p/{id}/signals", a.addP2PSignal)
	mux.HandleFunc("GET /v1/p2p/{id}/signals", a.p2pSignals)
	mux.HandleFunc("POST /v1/p2p/{id}/progress", a.p2pProgress)
	mux.HandleFunc("GET /v1/p2p/{id}/progress", a.p2pProgressSnapshot)
	mux.HandleFunc("POST /v1/p2p/{id}/complete", a.completeP2P)
	mux.HandleFunc("GET /v1/p2p/{id}/relay-transport", a.relayTransport)
	mux.HandleFunc("GET /v1/p2p/{id}/relay", a.relaySocket)
}

func (a *API) p2pProgressSnapshot(w http.ResponseWriter, r *http.Request) {
	snapshot, err := a.service.GetP2PProgress(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"))
	respond(w, snapshot, err)
}

func (a *API) createP2P(w http.ResponseWriter, r *http.Request) {
	var req service.P2PCreateRequest
	if !readRequest(w, r, &req) {
		return
	}
	transfer, err := a.service.CreateP2P(r.Context(), req)
	respond(w, transfer, err)
}

func (a *API) addP2PSignal(w http.ResponseWriter, r *http.Request) {
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
	signals, err := a.service.Signals(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"), parseInt64(r.URL.Query().Get("after")))
	respond(w, signals, err)
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
	respond(w, transfer, err)
}
