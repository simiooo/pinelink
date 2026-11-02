package integration

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type byteObservation struct {
	State        string     `json:"state"`
	Acknowledged int64      `json:"acknowledged_bytes"`
	Revision     int64      `json:"revision"`
	SampledAt    *time.Time `json:"sampled_at"`
}
type progressSnapshot struct {
	State        string          `json:"state"`
	StateVersion int64           `json:"state_version"`
	Delivery     byteObservation `json:"delivery"`
	Spool        byteObservation `json:"spool"`
	Route        struct {
		Kind  string `json:"kind"`
		Epoch int64  `json:"epoch"`
	} `json:"route"`
	Completion struct {
		Delivered bool `json:"delivered"`
		Stored    bool `json:"stored"`
	} `json:"completion"`
}

func progressRequest(t *testing.T, method, path string, body any, expected int, out any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, requiredEnv("PINELINK_TEST_SERVER")+path, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expected {
		t.Fatalf("%s %s: got %d, want %d: %s", method, path, response.StatusCode, expected, data)
	}
	if out != nil {
		if err = json.Unmarshal(data, out); err != nil {
			t.Fatalf("decode %s: %v", data, err)
		}
	}
}

func TestP2PProgressAuthorityAndRouteContinuity(t *testing.T) {
	id := uuid.NewString()
	sender, recipient := "progress-sender-"+id, "progress-recipient-"+id
	create := func() lifecycleTransfer {
		var tr lifecycleTransfer
		progressRequest(t, "POST", "/v1/p2p", map[string]any{"sender_owner_id": sender, "sender_device_id": "sender", "recipient_owner_id": recipient, "recipient_device_id": "receiver", "file_name": "test.bin", "size_bytes": 1000, "content_root": strings.Repeat("0", 64), "encryption_suite": "test", "encrypted_manifest": []byte("recipient envelope"), "sender_encrypted_manifest": []byte("sender envelope")}, 200, &tr)
		return tr
	}
	tr := create()
	actor := map[string]any{"owner_id": recipient, "device_id": "receiver", "expected_version": tr.Version}
	progressRequest(t, "POST", "/v1/transfers/"+tr.ID+"/accept", actor, 200, &tr)
	snap := func() progressSnapshot {
		var s progressSnapshot
		progressRequest(t, "GET", "/v1/p2p/"+tr.ID+"/progress?owner_id="+sender+"&device_id=sender", nil, 200, &s)
		return s
	}
	report := func(n int64, route string, epoch int64, want int) {
		progressRequest(t, "POST", "/v1/p2p/"+tr.ID+"/progress", map[string]any{"owner_id": recipient, "device_id": "receiver", "received_bytes": n, "route": route, "route_epoch": epoch}, want, nil)
	}
	complete := func(want int) {
		progressRequest(t, "POST", "/v1/p2p/"+tr.ID+"/complete", map[string]any{"owner_id": recipient, "device_id": "receiver", "expected_version": tr.Version}, want, nil)
	}
	complete(409)
	report(100, "DIRECT", 1, 200)
	report(200, "RELAY", 2, 200)
	report(250, "DIRECT", 1, 200)
	before := snap()
	if before.Delivery.Acknowledged != 250 || before.Route.Kind != "RELAY" || before.Route.Epoch != 2 || before.Delivery.Revision != 3 || before.StateVersion != tr.Version {
		t.Fatalf("invalid merged snapshot: %+v", before)
	}
	report(100, "DIRECT", 1, 200)
	after := snap()
	if after.Delivery.Revision != before.Delivery.Revision || after.Delivery.SampledAt == nil || !after.Delivery.SampledAt.Equal(*before.Delivery.SampledAt) {
		t.Fatalf("duplicate refreshed a byte observation: %+v / %+v", before, after)
	}
	report(300, "DIRECT", 2, 409)
	report(1001, "RELAY", 2, 400)
	progressRequest(t, "GET", "/v1/p2p/"+tr.ID+"/progress?owner_id=outsider&device_id=receiver", nil, 404, nil)
	progressRequest(t, "POST", "/v1/p2p/"+tr.ID+"/progress", map[string]any{"owner_id": sender, "device_id": "sender", "received_bytes": 1000}, 400, nil)
	pool, err := pgxpool.New(t.Context(), requiredEnv("PINELINK_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var task lifecycleTask
	if err = pool.QueryRow(t.Context(), `SELECT id,state,version,progress_bytes,total_bytes FROM tasks WHERE kind='DOWNLOAD' AND reference_id=$1`, tr.ID).Scan(&task.ID, &task.State, &task.Version, &task.ProgressBytes, &task.TotalBytes); err != nil {
		t.Fatal(err)
	}
	if task.ProgressBytes != 250 {
		t.Fatalf("task/transfer disagree: %+v", task)
	}
	progressRequest(t, "POST", "/v1/tasks/"+task.ID+"/pause", map[string]any{"owner_id": recipient, "device_id": "receiver", "expected_version": task.Version}, 200, &task)
	report(400, "RELAY", 2, 409)
	progressRequest(t, "POST", "/v1/tasks/"+task.ID+"/progress", map[string]any{"owner_id": recipient, "device_id": "receiver", "progress_bytes": 400}, 409, nil)
	complete(409)
	progressRequest(t, "POST", "/v1/tasks/"+task.ID+"/resume", map[string]any{"owner_id": recipient, "device_id": "receiver", "expected_version": task.Version}, 200, &task)
	var wg sync.WaitGroup
	for _, n := range []int64{300, 350, 400, 450, 500} {
		wg.Add(1)
		go func(n int64) { defer wg.Done(); report(n, "RELAY", 2, 200) }(n)
	}
	wg.Wait()
	if s := snap(); s.Delivery.Acknowledged != 500 {
		t.Fatalf("out of order progress regressed: %+v", s)
	}
	// The legacy task endpoint updates the same P2P authority, rather than a competing number.
	progressRequest(t, "POST", "/v1/tasks/"+task.ID+"/progress", map[string]any{"owner_id": recipient, "device_id": "receiver", "progress_bytes": 1000}, 200, nil)
	if s := snap(); s.Delivery.State != "VERIFYING" || s.Completion.Delivered || s.Completion.Stored {
		t.Fatalf("100%% invented completion: %+v", s)
	}
	complete(200)
	complete(200)
	if s := snap(); !s.Completion.Delivered || s.Completion.Stored || s.Delivery.State != "DELIVERED" {
		t.Fatalf("independent outcomes: %+v", s)
	}
	report(1000, "DIRECT", 3, 409)

	t.Run("cancel-vs-progress", func(t *testing.T) {
		for i := 0; i < 5; i++ {
			next := create()
			progressRequest(t, "POST", "/v1/transfers/"+next.ID+"/accept", map[string]any{"owner_id": recipient, "device_id": "receiver", "expected_version": 1}, 200, &next)
			gate := make(chan struct{})
			results := make(chan int, 2)
			for _, op := range []string{"cancel", "progress"} {
				go func(op string) {
					<-gate
					path := "/v1/transfers/" + next.ID + "/cancel"
					body := map[string]any{"owner_id": sender, "device_id": "sender", "expected_version": next.Version}
					if op == "progress" {
						path = "/v1/p2p/" + next.ID + "/progress"
						body = map[string]any{"owner_id": recipient, "device_id": "receiver", "received_bytes": 500}
					}
					encoded, _ := json.Marshal(body)
					response, err := http.Post(requiredEnv("PINELINK_TEST_SERVER")+path, "application/json", bytes.NewReader(encoded))
					if err != nil {
						results <- 0
						return
					}
					io.Copy(io.Discard, response.Body)
					response.Body.Close()
					results <- response.StatusCode
				}(op)
			}
			close(gate)
			for j := 0; j < 2; j++ {
				if code := <-results; code != 200 && code != 409 {
					t.Fatalf("race returned %d", code)
				}
			}
			var state, taskState string
			var bytesReceived, bytesTask int64
			if err := pool.QueryRow(t.Context(), `SELECT tr.state,t.state,tr.received_bytes,t.progress_bytes FROM transfer_requests tr JOIN tasks t ON t.reference_id=tr.id AND t.kind='DOWNLOAD' WHERE tr.id=$1`, next.ID).Scan(&state, &taskState, &bytesReceived, &bytesTask); err != nil {
				t.Fatal(err)
			}
			if state != "CANCELLED" || taskState != "CANCELLED" || bytesReceived != bytesTask {
				t.Fatalf("broken atomic race: %s %s %d %d", state, taskState, bytesReceived, bytesTask)
			}
		}
	})
	t.Run("expiry-before-worker", func(t *testing.T) {
		next := create()
		progressRequest(t, "POST", "/v1/transfers/"+next.ID+"/accept", map[string]any{"owner_id": recipient, "device_id": "receiver", "expected_version": 1}, 200, &next)
		if _, err = pool.Exec(t.Context(), `UPDATE transfer_requests SET expires_at=now()-interval '1 second' WHERE id=$1`, next.ID); err != nil {
			t.Fatal(err)
		}
		progressRequest(t, "POST", "/v1/p2p/"+next.ID+"/progress", map[string]any{"owner_id": recipient, "device_id": "receiver", "received_bytes": 1000}, 409, nil)
		var s progressSnapshot
		progressRequest(t, "GET", fmt.Sprintf("/v1/p2p/%s/progress?owner_id=%s&device_id=sender", next.ID, sender), nil, 200, &s)
		if s.Delivery.State != "EXPIRED" || s.Delivery.Acknowledged != 0 {
			t.Fatalf("deadline bypass: %+v", s)
		}
	})
}
