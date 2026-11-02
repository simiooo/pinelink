package httpapi

import "net/http"

func (a *API) registerProjection(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/tasks/{id}/projection", a.taskProjection)
}

func (a *API) taskProjection(w http.ResponseWriter, r *http.Request) {
	projection, err := a.service.GetTaskProjection(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"))
	respond(w, projection, err)
}
