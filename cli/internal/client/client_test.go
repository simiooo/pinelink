package client

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestConflictIncludesAuthoritativeStateWithoutDumpingResource(t *testing.T) {
	response := &http.Response{Status: "409 Conflict", Body: io.NopCloser(strings.NewReader(`{"code":"STATE_CONFLICT","message":"state version conflict","current":{"state":"CANCELLED","encrypted_manifest":"private envelope","file_name":"private filename"}}`))}
	message := responseError(response).Error()
	if !strings.Contains(message, "CANCELLED") || strings.Contains(message, "private") {
		t.Fatalf("bad error: %s", message)
	}
}
