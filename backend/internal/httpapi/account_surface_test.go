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
