package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSOnlyAllowsConfiguredOrigins(t *testing.T) {
	handler := withCORS(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }), []string{"http://127.0.0.1:5173"})
	request := httptest.NewRequest(http.MethodOptions, "/v1/tasks", nil)
	request.Header.Set("Origin", "http://127.0.0.1:5173")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNoContent || response.Header().Get("Access-Control-Allow-Origin") != "http://127.0.0.1:5173" {
		t.Fatalf("expected allowed preflight: %d %v", response.Code, response.Header())
	}

	request = httptest.NewRequest(http.MethodGet, "/v1/tasks", nil)
	request.Header.Set("Origin", "https://untrusted.example")
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatal("untrusted origin must not be allowed")
	}
}
