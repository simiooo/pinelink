package httpapi

import (
	"net/http"

	"github.com/simiooo/pinelink/backend/internal/service"
)

func (a *API) registerStorageLifecycle(mux *http.ServeMux) {
	a.registerManifests(mux)
	mux.HandleFunc("POST /v1/devices/register", a.registerDevice)
	mux.HandleFunc("POST /v1/devices/revoke", a.revokeDevice)
	mux.HandleFunc("GET /v1/devices", a.listDevices)
	mux.HandleFunc("GET /v1/devices/{targetOwner}/{targetDevice}/public-key", a.recipientPublicKey)
	mux.HandleFunc("POST /v1/broadcasts", a.createBroadcast)
	mux.HandleFunc("GET /v1/files/{id}", a.getFile)
	mux.HandleFunc("POST /v1/files/{id}/promote", a.promoteFile)
	mux.HandleFunc("POST /v1/files/{id}/delete", a.deleteFile)
	mux.HandleFunc("GET /v1/quota", a.getQuota)
}

func (a *API) recipientPublicKey(w http.ResponseWriter, r *http.Request) {
	key, err := a.service.GetRecipientPublicKey(r.Context(), r.PathValue("targetOwner"), r.PathValue("targetDevice"))
	respond(w, key, err)
}

func (a *API) registerDevice(w http.ResponseWriter, r *http.Request) {
	var req service.DeviceRequest
	if !readRequest(w, r, &req) {
		return
	}
	device, err := a.service.RegisterDevice(r.Context(), req)
	respond(w, device, err)
}

func (a *API) revokeDevice(w http.ResponseWriter, r *http.Request) {
	var req service.DeviceRequest
	if !readRequest(w, r, &req) {
		return
	}
	device, err := a.service.RevokeDevice(r.Context(), req)
	respond(w, device, err)
}

func (a *API) listDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := a.service.ListDevices(r.Context(), r.URL.Query().Get("owner_id"))
	respond(w, devices, err)
}

func (a *API) createBroadcast(w http.ResponseWriter, r *http.Request) {
	var req service.BroadcastRequest
	if !readRequest(w, r, &req) {
		return
	}
	transfers, err := a.service.CreateBroadcast(r.Context(), req)
	respond(w, transfers, err)
}

func (a *API) getFile(w http.ResponseWriter, r *http.Request) {
	if tr := r.URL.Query().Get("transfer_id"); tr != "" {
		file, err := a.service.GetGrantedFile(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"), tr)
		respond(w, file, err)
		return
	}
	file, err := a.service.GetFile(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"))
	respond(w, file, err)
}

func (a *API) promoteFile(w http.ResponseWriter, r *http.Request) {
	var req service.FileActionRequest
	if !readRequest(w, r, &req) {
		return
	}
	file, err := a.service.PromoteFile(r.Context(), r.PathValue("id"), req)
	respond(w, file, err)
}

func (a *API) deleteFile(w http.ResponseWriter, r *http.Request) {
	var req service.FileActionRequest
	if !readRequest(w, r, &req) {
		return
	}
	file, err := a.service.DeleteFile(r.Context(), r.PathValue("id"), req)
	respond(w, file, err)
}

func (a *API) getQuota(w http.ResponseWriter, r *http.Request) {
	quota, err := a.service.GetQuota(r.Context(), r.URL.Query().Get("scope_id"))
	respond(w, quota, err)
}
