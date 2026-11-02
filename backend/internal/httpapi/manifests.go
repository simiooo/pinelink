package httpapi

import (
	"net/http"

	"github.com/simiooo/pinelink/backend/internal/service"
)

func (a *API) registerManifests(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/files/{id}/manifest", a.getManifest)
	mux.HandleFunc("POST /v1/files/{id}/envelopes", a.putManifestEnvelope)
}

func (a *API) getManifest(w http.ResponseWriter, r *http.Request) {
	envelope, err := a.service.GetManifest(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"), r.URL.Query().Get("transfer_id"))
	respond(w, envelope, err)
}

func (a *API) putManifestEnvelope(w http.ResponseWriter, r *http.Request) {
	var req service.PutEnvelopeRequest
	if !readRequest(w, r, &req) {
		return
	}
	if err := a.service.PutManifestEnvelope(r.Context(), r.PathValue("id"), req); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "stored"})
}
