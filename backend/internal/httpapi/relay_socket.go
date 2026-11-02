package httpapi

import (
	"net/http"

	"github.com/coder/websocket"
)

func (a *API) relayTransport(w http.ResponseWriter, r *http.Request) {
	if _, err := a.service.GetTransfer(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id")); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"transport": a.service.RelayTransport()})
}

func (a *API) relaySocket(w http.ResponseWriter, r *http.Request) {
	if a.service.RelayTransport() != "websocket" {
		http.NotFound(w, r)
		return
	}
	ctx, finish, ok := a.beginRelay(r.Context())
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "SERVER_STOPPING"})
		return
	}
	defer finish()
	select {
	case a.relaySlots <- struct{}{}:
		defer func() { <-a.relaySlots }()
	default:
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"code": "RELAY_BUSY", "message": "relay busy"})
		return
	}
	session, err := a.service.PrepareRelaySocket(ctx, r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"), r.URL.Query().Get("role"))
	if err != nil {
		writeError(w, err)
		return
	}
	// Default same-origin checks remain enabled; a reverse proxy must preserve Host.
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	_ = session.Run(ctx, conn)
}
