package httpapi

import "net/http"

func (a *API) registerPullLookup(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/pulls/{id}", a.getPull)
}

func (a *API) getPull(w http.ResponseWriter, r *http.Request) {
	pull, err := a.service.GetPull(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"))
	respond(w, pull, err)
}
