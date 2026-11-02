package integration

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestSyncParticipantIsolation(t *testing.T) {
	owner := "sync-" + uuid.NewString()
	var seed struct {
		CurrentSequence int64 `json:"current_sequence"`
	}
	progressRequest(t, "GET", "/v1/sync?owner_id="+owner+"&device_id=shared-device&after=-1", nil, 200, &seed)
	var foreign lifecycleTransfer
	progressRequest(t, "POST", "/v1/p2p", map[string]any{"sender_owner_id": "foreign-" + owner, "sender_device_id": "shared-device", "recipient_owner_id": "other-" + owner, "recipient_device_id": "shared-device", "file_name": "private.bin", "size_bytes": 10, "content_root": strings.Repeat("0", 64), "encryption_suite": "test", "encrypted_manifest": []byte("private"), "sender_encrypted_manifest": []byte("private")}, 200, &foreign)
	var result struct {
		Events []struct {
			AggregateID string `json:"aggregate_id"`
		} `json:"events"`
	}
	progressRequest(t, "GET", "/v1/sync?owner_id="+owner+"&device_id=shared-device&after=0", nil, 200, &result)
	for _, e := range result.Events {
		if e.AggregateID == foreign.ID {
			t.Fatal("shared device ID leaked another owner's event")
		}
	}
	progressRequest(t, "GET", "/v1/sync", nil, 400, nil)
}

func TestSignedSyncCannotOmitOrSpoofQueryIdentity(t *testing.T) {
	owner := "signed-sync-" + uuid.NewString()
	device := "shared-device"
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pool, err := pgxpool.New(t.Context(), requiredEnv("PINELINK_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if _, err = pool.Exec(t.Context(), `INSERT INTO devices(owner_id,device_id,status,signing_public_key) VALUES($1,$2,'ACTIVE',$3)`, owner, device, base64.StdEncoding.EncodeToString(pub)); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/v1/sync?after=0", "/v1/sync?after=0&owner_id=someone-else&device_id=shared-device"} {
		req, err := http.NewRequestWithContext(t.Context(), "GET", requiredEnv("PINELINK_TEST_AUTH_SERVER")+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		stamp := strconv.FormatInt(time.Now().Unix(), 10)
		digest := sha256.Sum256(nil)
		payload := "GET\n" + req.URL.RequestURI() + "\n" + stamp + "\n" + hex.EncodeToString(digest[:])
		req.Header.Set("X-Pinelink-Owner", owner)
		req.Header.Set("X-Pinelink-Device", device)
		req.Header.Set("X-Pinelink-Timestamp", stamp)
		req.Header.Set("X-Pinelink-Signature", base64.StdEncoding.EncodeToString(ed25519.Sign(key, []byte(payload))))
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var result struct {
			Events []json.RawMessage `json:"events"`
		}
		err = json.NewDecoder(response.Body).Decode(&result)
		response.Body.Close()
		if response.StatusCode != 200 || err != nil {
			t.Fatalf("signed sync failed: status=%d err=%v", response.StatusCode, err)
		}
		if len(result.Events) != 0 {
			t.Fatalf("query identity leaked %d foreign events", len(result.Events))
		}
	}
}
