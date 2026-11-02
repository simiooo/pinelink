package httpapi

import (
	"net/http"
	"strconv"
	"time"

	"github.com/simiooo/pinelink/backend/internal/service"
)

func (a *API) registerLifecycle(mux *http.ServeMux) {
	a.registerPullLookup(mux)
	a.registerStorageLifecycle(mux)
	a.registerP2P(mux)
	a.registerProjection(mux)
	mux.HandleFunc("POST /v1/transfers", a.createTransfer)
	mux.HandleFunc("GET /v1/transfers", a.listTransfers)
	mux.HandleFunc("GET /v1/transfers/{id}", a.getTransfer)
	mux.HandleFunc("POST /v1/transfers/{id}/{action}", a.transferAction)
	mux.HandleFunc("GET /v1/notifications", a.notifications)
	mux.HandleFunc("GET /v1/tasks", a.listTasks)
	mux.HandleFunc("GET /v1/tasks/{id}", a.getTask)
	mux.HandleFunc("POST /v1/tasks/{id}/progress", a.taskProgress)
	mux.HandleFunc("POST /v1/tasks/{id}/{action}", a.taskAction)
	mux.HandleFunc("GET /v1/sync", a.syncEvents)
}

func (a *API) createTransfer(w http.ResponseWriter, r *http.Request) {
	var req service.TransferCreateRequest
	if !readRequest(w, r, &req) {
		return
	}
	transfer, err := a.service.CreateTransfer(r.Context(), req)
	respond(w, transfer, err)
}

func (a *API) getTransfer(w http.ResponseWriter, r *http.Request) {
	transfer, err := a.service.GetTransfer(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"))
	respond(w, transfer, err)
}

func (a *API) listTransfers(w http.ResponseWriter, r *http.Request) {
	before := parseTime(r.URL.Query().Get("before"))
	transfers, err := a.service.ListTransfers(r.Context(), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"),
		r.URL.Query().Get("role"), r.URL.Query().Get("state"), before, parseInt(r.URL.Query().Get("limit")))
	respond(w, transfers, err)
}

func (a *API) notifications(w http.ResponseWriter, r *http.Request) {
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "PENDING"
	}
	before := parseTime(r.URL.Query().Get("before"))
	transfers, err := a.service.ListTransfers(r.Context(), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"),
		"recipient", state, before, parseInt(r.URL.Query().Get("limit")))
	respond(w, transfers, err)
}

func (a *API) transferAction(w http.ResponseWriter, r *http.Request) {
	var req service.TransferActionRequest
	if !readRequest(w, r, &req) {
		return
	}
	transfer, err := a.service.RespondTransfer(r.Context(), r.PathValue("id"), r.PathValue("action"), req)
	respond(w, transfer, err)
}

func (a *API) getTask(w http.ResponseWriter, r *http.Request) {
	task, err := a.service.GetTask(r.Context(), r.PathValue("id"), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"))
	respond(w, task, err)
}

func (a *API) listTasks(w http.ResponseWriter, r *http.Request) {
	includeDeleted, _ := strconv.ParseBool(r.URL.Query().Get("include_deleted"))
	tasks, err := a.service.ListTasks(r.Context(), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"),
		r.URL.Query().Get("kind"), r.URL.Query().Get("state"), includeDeleted, parseTime(r.URL.Query().Get("before")), parseInt(r.URL.Query().Get("limit")))
	respond(w, tasks, err)
}

func (a *API) taskAction(w http.ResponseWriter, r *http.Request) {
	var req service.TaskActionRequest
	if !readRequest(w, r, &req) {
		return
	}
	task, err := a.service.ControlTask(r.Context(), r.PathValue("id"), r.PathValue("action"), req)
	respond(w, task, err)
}

func (a *API) taskProgress(w http.ResponseWriter, r *http.Request) {
	var req service.TaskProgressRequest
	if !readRequest(w, r, &req) {
		return
	}
	task, err := a.service.ReportTaskProgress(r.Context(), r.PathValue("id"), req)
	respond(w, task, err)
}

func (a *API) syncEvents(w http.ResponseWriter, r *http.Request) {
	events, err := a.service.SyncEvents(r.Context(), parseInt64(r.URL.Query().Get("after")), parseInt(r.URL.Query().Get("limit")), r.URL.Query().Get("owner_id"), r.URL.Query().Get("device_id"))
	respond(w, events, err)
}

func readRequest(w http.ResponseWriter, r *http.Request, destination any) bool {
	if err := decodeJSON(r, destination); err != nil {
		writeError(w, err)
		return false
	}
	return true
}

func respond(w http.ResponseWriter, value any, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, value)
}

func parseTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil
	}
	return &parsed
}

func parseInt(value string) int {
	parsed, _ := strconv.Atoi(value)
	return parsed
}

func parseInt64(value string) int64 {
	parsed, _ := strconv.ParseInt(value, 10, 64)
	return parsed
}
