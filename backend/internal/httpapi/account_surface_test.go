package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBrowserSurfacesAndNativeTokenRemainIndependent(t *testing.T) {
	r := httptest.NewRequest("GET", "https://pinelink.example/v1/auth/session", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "react-session"})
	r.AddCookie(&http.Cookie{Name: flutterSessionCookie, Value: "flutter-session"})
	if accountToken(r) != "react-session" {
		t.Fatal("React selected another surface")
	}
	r.Header.Set("X-Pinelink-Surface", "app")
	if accountToken(r) != "flutter-session" {
		t.Fatal("Flutter selected another surface")
	}
	r.Header.Set("X-Pinelink-Session", "native-session")
	if accountToken(r) != "native-session" {
		t.Fatal("native credential was replaced by a cookie")
	}
}

func TestGuestAndAccountCookiesNeverSelectEachOther(t *testing.T) {
	r := httptest.NewRequest("GET", "https://pinelink.example/v1/transfers", nil)
	r.AddCookie(&http.Cookie{Name: sessionCookie, Value: "account"})
	r.AddCookie(&http.Cookie{Name: "__Host-pinelink_guest", Value: "guest"})
	r.AddCookie(&http.Cookie{Name: "__Host-pinelink_flutter_guest", Value: "flutter-guest"})
	if accountToken(r) != "account" || guestToken(r) != "guest" {
		t.Fatal("guest replaced the account credential")
	}
	r.Header.Set("X-Pinelink-Surface", "app")
	if guestToken(r) != "flutter-guest" || accountToken(r) != "" {
		t.Fatal("surface cookie isolation failed")
	}
	r.Header.Set("X-Pinelink-Guest-Session", "native-guest")
	if guestToken(r) != "native-guest" || accountToken(r) != "" {
		t.Fatal("native guest token entered the account slot")
	}
}

func TestGuestRouteScopeOnlyAddsBoundedSignallingAndDeniesUnknownOperations(t *testing.T) {
	for _, test := range []struct {
		method, path string
		allowed      bool
	}{
		{"POST", "/v1/guest/invites", true}, {"POST", "/v1/guest/candidates/id/approve", true},
		{"GET", "/v1/guest/connections", true}, {"POST", "/v1/p2p", true},
		{"DELETE", "/v1/guest/connections/id", true}, {"PATCH", "/v1/guest/connections/id", false},
		{"DELETE", "/v1/guest/connections/id/future-action", false},
		{"POST", "/v1/p2p/id/relay-ticket", true}, {"GET", "/v1/p2p/id/relay?role=receiver", true},
		{"POST", "/v1/p2p/id/progress", true}, {"POST", "/v1/transfers/id/accept", true}, {"POST", "/v1/tasks/id/resume", true},
		{"POST", "/v1/p2p/id/signals", true}, {"GET", "/v1/p2p/id/signals", true},
		{"PATCH", "/v1/p2p/id/signals", false}, {"DELETE", "/v1/p2p/id/signals", false}, {"POST", "/v1/p2p/id/signals/future", false},
		{"POST", "/v1/uploads/prepare", false}, {"POST", "/v1/transfers", false}, {"POST", "/v1/tasks/id/delete", false},
		{"POST", "/v1/p2p/id/future-action", false}, {"POST", "/v1/guest/future-action", false}, {"DELETE", "/v1/tasks/id", false},
	} {
		r := httptest.NewRequest(test.method, "https://pinelink.example"+test.path, nil)
		if got := guestRouteAllowed(r); got != test.allowed {
			t.Errorf("%s %s: allowed=%v", test.method, test.path, got)
		}
	}
}
