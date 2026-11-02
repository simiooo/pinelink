package p2p

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/simiooo/pinelink/cli/internal/client"
)

func TestSocketSenderKeepsLiveUploadThroughTransientControlFailure(t *testing.T) {
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusUnauthorized} {
		t.Run(fmt.Sprint(status), func(t *testing.T) {
			var polls atomic.Int32
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			mux := http.NewServeMux()
			// DialRelay uses the same signed HTTP transport as normal API calls.
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/progress") {
					if polls.Add(1) <= 2 || status == http.StatusUnauthorized {
						http.Error(w, "temporary control response", status)
						return
					}
					w.Header().Set("Content-Type", "application/json")
					fmt.Fprint(w, `{"state":"PENDING","completion":{"stored":true}}`)
					return
				}
				if strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
					conn, err := websocket.Accept(w, r, nil)
					if err != nil {
						return
					}
					defer conn.CloseNow()
					for {
						if _, _, err = conn.Read(ctx); err != nil {
							return
						}
					}
				}
				http.NotFound(w, r)
			})
			server := httptest.NewServer(mux)
			defer server.Close()
			api, err := client.New(server.URL, nil, "sender", "device")
			if err != nil {
				t.Fatal(err)
			}
			input := t.TempDir() + "/empty"
			if err := os.WriteFile(input, nil, 0o600); err != nil {
				t.Fatal(err)
			}
			err = runSocketSender(ctx, api, client.Transfer{ID: "transfer", SenderOwnerID: "sender", SenderDeviceID: "device", ExpiresAt: time.Now().Add(time.Minute)}, input)
			if status == http.StatusServiceUnavailable {
				if err != nil || polls.Load() != 3 {
					t.Fatal("transient failure cancelled upload", err, polls.Load())
				}
			} else if err == nil || polls.Load() != 1 {
				t.Fatal("permission failure was retried", err, polls.Load())
			}
		})
	}
}

func TestRelayPollRetriesOnlyTransientFailures(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409, 410, 408, 429, 500, 502, 503} {
		want := status == 408 || status == 429 || status >= 500
		if got := transientRelayPoll(&client.ResponseError{StatusCode: status}); got != want {
			t.Fatal(status, got)
		}
	}
	if !transientRelayPoll(&net.OpError{Op: "read", Net: "tcp", Err: context.DeadlineExceeded}) {
		t.Fatal("network timeout not retried")
	}
}
