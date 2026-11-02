package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	khttp "github.com/go-kratos/kratos/v2/transport/http"

	"github.com/simiooo/pinelink/backend/internal/service"
)

type API struct {
	service            *service.Service
	logger             *log.Helper
	deviceAuthRequired bool
}

// Set by the release build; /healthz also identifies the actual deployed binary.
var BuildVersion = "dev"

func NewServer(addr string, svc *service.Service, logger log.Logger, deviceAuthRequired bool, corsOrigins ...[]string) *khttp.Server {
	api := &API{service: svc, logger: log.NewHelper(logger), deviceAuthRequired: deviceAuthRequired}
	var origins []string
	if len(corsOrigins) > 0 {
		origins = corsOrigins[0]
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", api.health)
	mux.HandleFunc("POST /v1/uploads/prepare", api.prepare)
	mux.HandleFunc("PUT /v1/uploads/{uploadID}/chunks/{index}", api.uploadChunk)
	mux.HandleFunc("POST /v1/uploads/{uploadID}/complete", api.completeUpload)
	mux.HandleFunc("POST /v1/uploads/reuse/commit", api.commitReuse)
	mux.HandleFunc("POST /v1/files/{entryID}/pulls", api.createPull)
	mux.HandleFunc("GET /v1/pulls/{pullID}/content", api.content)
	mux.HandleFunc("POST /v1/pulls/{pullID}/complete", api.completePull)
	api.registerLifecycle(mux)
	api.registerReplication(mux)

	server := khttp.NewServer(
		khttp.Address(addr),
		// Storage operations enforce their own bounded I/O leases. Kratos' 1s
		// default cancels a valid seal/stream once it spans several chunks.
		khttp.Timeout(0),
		khttp.Middleware(recovery.Recovery(), logging.Server(logger)),
	)
	server.HandlePrefix("/", withCORS(api.withDeviceAuth(mux), origins))
	return server
}

func (a *API) health(w http.ResponseWriter, r *http.Request) {
	if err := a.service.Ready(r.Context()); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": BuildVersion})
}

func (a *API) prepare(w http.ResponseWriter, r *http.Request) {
	var req service.PrepareRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	response, err := a.service.PrepareUpload(r.Context(), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *API) uploadChunk(w http.ResponseWriter, r *http.Request) {
	index, err := strconv.Atoi(r.PathValue("index"))
	if err != nil {
		writeError(w, fmt.Errorf("%w: invalid chunk index", service.ErrInvalid))
		return
	}
	body := http.MaxBytesReader(w, r.Body, 16<<20+1)
	data, err := io.ReadAll(body)
	if err != nil {
		writeError(w, fmt.Errorf("%w: read chunk body", service.ErrInvalid))
		return
	}
	if err := a.service.UploadChunk(r.Context(), r.PathValue("uploadID"), index, r.Header.Get("X-Chunk-SHA256"), data); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "received"})
}

func (a *API) completeUpload(w http.ResponseWriter, r *http.Request) {
	response, err := a.service.CompleteUpload(r.Context(), r.PathValue("uploadID"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *API) commitReuse(w http.ResponseWriter, r *http.Request) {
	var req service.ReuseCommitRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	response, err := a.service.CommitReuse(r.Context(), req.ReuseToken)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (a *API) createPull(w http.ResponseWriter, r *http.Request) {
	var req service.PullCreateRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	response, err := a.service.CreatePull(r.Context(), r.PathValue("entryID"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, response)
}

func (a *API) completePull(w http.ResponseWriter, r *http.Request) {
	var req service.PullCompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, err)
		return
	}
	response, err := a.service.CompletePull(r.Context(), r.PathValue("pullID"), req)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

type byteRange struct {
	start int64
	end   int64
}

func (a *API) content(w http.ResponseWriter, r *http.Request) {
	content, lease, err := a.service.OpenPullRead(r.Context(), r.PathValue("pullID"))
	if err != nil {
		writeError(w, err)
		return
	}
	defer a.service.CloseRead(r.Context(), lease)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("ETag", `"`+content.Pull.ContentRoot+`"`)
	w.Header().Set("X-Pinelink-Content-Root", content.Pull.ContentRoot)

	start, end := int64(0), content.TotalSize-1
	status := http.StatusOK
	if value := r.Header.Get("Range"); value != "" {
		start, end, err = parseByteRange(value, content.TotalSize)
		if err != nil {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", content.TotalSize))
			writeJSON(w, http.StatusRequestedRangeNotSatisfiable, map[string]string{"code": "RANGE_INVALID", "message": err.Error()})
			return
		}
		status = http.StatusPartialContent
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, content.TotalSize))
	}
	length := int64(0)
	if content.TotalSize > 0 {
		length = end - start + 1
	}
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.WriteHeader(status)
	if length == 0 {
		return
	}
	if err := a.writeChunks(w, r, content, start, end, lease); err != nil {
		a.logger.Errorf("stream pull %s: %v", content.Pull.ID, err)
	}
}

func (a *API) writeChunks(w http.ResponseWriter, r *http.Request, content service.Content, start, end int64, lease string) error {
	first := start / int64(content.ChunkSize)
	last := end / int64(content.ChunkSize)
	for index := first; index <= last; index++ {
		chunk, err := a.service.PullChunk(r.Context(), content, index)
		if err != nil {
			return err
		}
		chunkStart := index * int64(content.ChunkSize)
		from := max(start, chunkStart) - chunkStart
		to := min(end, chunkStart+chunk.Size-1) - chunkStart
		if err := a.service.CheckPullRead(r.Context(), content.Pull.ID, lease); err != nil {
			return err
		}
		read, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		object, err := a.service.GetBlob(read, chunk.Key, from, to)
		if err != nil {
			cancel()
			return err
		}
		buffer := make([]byte, 64<<10)
		left := to - from + 1
		var copyErr error
		for left > 0 {
			n, e := io.ReadFull(object, buffer[:min(int64(len(buffer)), left)])
			if e != nil {
				copyErr = e
				break
			}
			if e = a.service.CheckPullRead(r.Context(), content.Pull.ID, lease); e != nil {
				copyErr = e
				break
			}
			_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(30 * time.Second))
			written, e := w.Write(buffer[:n])
			if e != nil {
				copyErr = e
				break
			}
			if written != n {
				copyErr = io.ErrShortWrite
				break
			}
			left -= int64(n)
		}
		closeErr := object.Close()
		cancel()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (a *API) serviceBlob(r *http.Request, key string, start, end int64) (io.ReadCloser, error) {
	return a.service.GetBlob(r.Context(), key, start, end)
}

func parseByteRange(value string, size int64) (int64, int64, error) {
	if size <= 0 || !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return 0, 0, fmt.Errorf("only a single range of a non-empty file is supported")
	}
	parts := strings.SplitN(strings.TrimPrefix(value, "bytes="), "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("malformed range")
	}
	if parts[0] == "" {
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, fmt.Errorf("malformed suffix range")
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, nil
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, fmt.Errorf("range start outside file")
	}
	end := size - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return 0, 0, fmt.Errorf("malformed range end")
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, nil
}

func decodeJSON(r *http.Request, destination any) error {
	limit := int64(1 << 20)
	if r.URL.Path == "/v1/replication/events" {
		limit = 12 << 20
	}
	decoder := json.NewDecoder(http.MaxBytesReader(nil, r.Body, limit))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("%w: %v", service.ErrInvalid, err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("%w: request must contain one JSON value", service.ErrInvalid)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusInternalServerError, "INTERNAL", "internal error"
	var current any
	var resource string
	var stateConflict *service.StateConflictError
	switch {
	case errors.As(err, &stateConflict):
		status, code, message = http.StatusConflict, "STATE_CONFLICT", stateConflict.Error()
		current, resource = stateConflict.Current, stateConflict.Resource
	case errors.Is(err, service.ErrUnauthorized):
		status, code, message = http.StatusUnauthorized, "DEVICE_UNAUTHORIZED", "device authentication failed"
	case errors.Is(err, service.ErrInvalid):
		status, code, message = http.StatusBadRequest, "INVALID_REQUEST", err.Error()
	case errors.Is(err, service.ErrNotFound):
		status, code, message = http.StatusNotFound, "NOT_FOUND", "resource not found"
	case errors.Is(err, service.ErrConflict), errors.Is(err, service.ErrIncomplete), errors.Is(err, service.ErrReuseVerified):
		status, code, message = http.StatusConflict, "CONFLICT", err.Error()
	case errors.Is(err, service.ErrExpired):
		status, code, message = http.StatusGone, "EXPIRED", "resource expired"
	case errors.Is(err, service.ErrChecksum):
		status, code, message = http.StatusBadRequest, "CHECKSUM_MISMATCH", "checksum mismatch"
	}
	if status == http.StatusInternalServerError {
		log.NewHelper(log.DefaultLogger).Error(err)
	}
	body := map[string]any{"code": code, "message": message}
	if current != nil {
		body["resource"] = resource
		body["current"] = current
	}
	writeJSON(w, status, body)
}
