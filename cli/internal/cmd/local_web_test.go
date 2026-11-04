package cmd

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLocalWebBundleRemainsLocalAndDoesNotProxyCloud(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<!doctype html>local graph"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sw.js"), []byte("// local worker"), 0600); err != nil {
		t.Fatal(err)
	}
	h, err := localWebHandler(dir, "127.0.0.1:17643")
	if err != nil {
		t.Fatal(err)
	}
	request := func(method, host, p, origin string) *httptest.ResponseRecorder {
		q := httptest.NewRequest(method, "http://"+host+p, nil)
		if origin != "" {
			q.Header.Set("Origin", origin)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, q)
		return w
	}
	for _, p := range []string{"/", "/files", "/connect", "/sw.js"} {
		w := request("GET", "127.0.0.1:17643", p, "")
		if w.Code != 200 || w.Header().Get("X-Frame-Options") != "DENY" {
			t.Fatalf("local resource %s: %d", p, w.Code)
		}
	}
	w := request("GET", "127.0.0.1:17643", "/api/v1/auth/capabilities", "")
	var cap map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &cap); err != nil || cap["local_only"] != true || cap["enabled"] != false {
		t.Fatalf("unexpected local capability: %v", cap)
	}
	for _, c := range []struct {
		method, host, path, origin string
		status                     int
	}{{"GET", "attacker.invalid:17643", "/", "", 403}, {"GET", "127.0.0.1:17643", "/", "https://attacker.invalid", 403}, {"POST", "127.0.0.1:17643", "/api/v1/p2p", "", 405}, {"GET", "127.0.0.1:17643", "/api/v1/transfers", "", 404}, {"GET", "127.0.0.1:17643", "/../secret", "", 404}} {
		w := request(c.method, c.host, c.path, c.origin)
		if w.Code != c.status {
			t.Fatalf("%s %s: %d", c.method, c.path, w.Code)
		}
		if strings.Contains(w.Body.String(), "secret value") {
			t.Fatal("escaped bundle root")
		}
	}
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err = os.WriteFile(outside, []byte("secret value"), 0600); err != nil {
		t.Fatal(err)
	}
	if os.Symlink(outside, filepath.Join(dir, "outside.txt")) == nil {
		h, err = localWebHandler(dir, "127.0.0.1:17643")
		if err != nil {
			t.Fatal(err)
		}
		if w := request("GET", "127.0.0.1:17643", "/outside.txt", ""); w.Code != 404 {
			t.Fatal("served an external symlink")
		}
	}
}
