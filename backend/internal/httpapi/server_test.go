package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRevocationNeverRebindsAnotherTargetToTheCaller(t *testing.T) {
	if !revocationTargetsSelf([]byte(`{"device_id":"self"}`), "self") {
		t.Fatal("self revocation denied")
	}
	for _, body := range []string{`{"device_id":"other"}`, `{}`, `{"device_id":null}`} {
		if revocationTargetsSelf([]byte(body), "self") {
			t.Fatal("another or missing target would revoke the caller")
		}
	}
}

func TestSignedReplicationBodyPreservesLargeCountersAndBatch(t *testing.T) {
	input := []byte(`{"owner_id":"spoofed","events":[{"source_sequence":9007199254740993,"ciphertext":"` + strings.Repeat("A", 2<<20) + `"}]}`)
	bound, err := bindAuthenticatedJSON(input, "verified-owner", "verified-device")
	if err != nil {
		t.Fatal(err)
	}
	var value struct {
		Owner  string `json:"owner_id"`
		Events []struct {
			Sequence   int64  `json:"source_sequence"`
			Ciphertext string `json:"ciphertext"`
		} `json:"events"`
	}
	r := httptest.NewRequest("POST", "/v1/replication/events", bytes.NewReader(bound))
	if err = decodeJSON(r, &value); err != nil || value.Owner != "verified-owner" || value.Events[0].Sequence != 9007199254740993 {
		t.Fatal("authenticated replication batch changed or was truncated", err)
	}
	var decoded any
	if err = json.Unmarshal(bound, &decoded); err != nil {
		t.Fatal(err)
	}
	r = httptest.NewRequest("POST", "/v1/devices/register", bytes.NewReader(bound))
	if err = decodeJSON(r, &value); err == nil {
		t.Fatal("ordinary request exceeded its body bound")
	}
}

func TestParseByteRange(t *testing.T) {
	tests := []struct {
		value      string
		start, end int64
		valid      bool
	}{
		{value: "bytes=0-99", start: 0, end: 99, valid: true},
		{value: "bytes=100-", start: 100, end: 999, valid: true},
		{value: "bytes=-100", start: 900, end: 999, valid: true},
		{value: "bytes=900-2000", start: 900, end: 999, valid: true},
		{value: "bytes=1000-", valid: false},
		{value: "bytes=0-1,4-5", valid: false},
		{value: "items=0-1", valid: false},
	}
	for _, test := range tests {
		start, end, err := parseByteRange(test.value, 1000)
		if test.valid && (err != nil || start != test.start || end != test.end) {
			t.Errorf("%s: got %d-%d, %v", test.value, start, end, err)
		}
		if !test.valid && err == nil {
			t.Errorf("%s: expected error", test.value)
		}
	}
}
