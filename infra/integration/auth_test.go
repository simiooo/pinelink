package integration

import (
	"bytes"
	"crypto/sha256"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

func TestDeviceAuthenticationAndCiphertext(t *testing.T) {
	h := &harness{t: t, server: requiredEnv("PINELINK_TEST_AUTH_SERVER"), cli: requiredEnv("PINELINK_TEST_CLI"),
		scope: "auth-" + uuid.NewString(), domain: "auth-domain", owner: "auth-owner", device: "auth-device", dedupSecret: "auth-secret"}
	if _, err := h.command("device", "register"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.command("quota"); err != nil {
		t.Fatalf("signed request failed: %v", err)
	}
	response, err := http.Get(h.server + "/v1/quota?scope_id=unsigned")
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unsigned request returned %s", response.Status)
	}

	directory := t.TempDir()
	input := filepath.Join(directory, "secret.txt")
	plaintext := []byte("plaintext-that-must-not-appear-in-cloud-storage")
	if err := os.WriteFile(input, plaintext, 0o600); err != nil {
		t.Fatal(err)
	}
	uploaded := h.upload(input, "PERSISTENT")
	output := filepath.Join(directory, "output.txt")
	h.download(uploaded.EntryID, output, false)
	assertFileEquals(input, output, t)

	pool, err := pgxpool.New(t.Context(), requiredEnv("PINELINK_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var objectKey string
	if err := pool.QueryRow(t.Context(), `
		SELECT c.object_key FROM upload_chunks c
		JOIN upload_sessions u ON u.id=c.upload_id
		JOIN file_entries e ON e.version_id=u.version_id
		WHERE e.id=$1 LIMIT 1
	`, uploaded.EntryID).Scan(&objectKey); err != nil {
		t.Fatal(err)
	}
	objects, err := minio.New(requiredEnv("PINELINK_S3_ENDPOINT"), &minio.Options{
		Creds: credentials.NewStaticV4(requiredEnv("PINELINK_S3_ACCESS_KEY"), requiredEnv("PINELINK_S3_SECRET_KEY"), ""),
	})
	if err != nil {
		t.Fatal(err)
	}
	object, err := objects.GetObject(t.Context(), requiredEnv("PINELINK_S3_BUCKET"), objectKey, minio.GetObjectOptions{})
	if err != nil {
		t.Fatal(err)
	}
	stored, err := io.ReadAll(object)
	_ = object.Close()
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stored, plaintext) {
		t.Fatal("object storage contains plaintext")
	}
	digest := sha256.Sum256(plaintext)
	var envelope []byte
	if err := pool.QueryRow(t.Context(), `SELECT ciphertext FROM manifest_envelopes WHERE version_id=$1 AND owner_id=$2 AND device_id=$3`, uploaded.VersionID, h.owner, h.device).Scan(&envelope); err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(envelope, plaintext) || bytes.Contains(envelope, digest[:]) {
		t.Fatal("manifest envelope exposes plaintext or its hash")
	}

	if _, err := h.command("device", "revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.command("quota"); err == nil {
		t.Fatal("revoked device was still authorized")
	}
	if _, err := h.command("device", "register"); err != nil {
		t.Fatalf("re-registration with the original key failed: %v", err)
	}
	if _, err := h.command("quota"); err != nil {
		t.Fatalf("re-registered device was not authorized: %v", err)
	}

	request, _ := http.NewRequest(http.MethodGet, h.server+"/v1/quota?scope_id=tampered", nil)
	request.Header.Set("X-Pinelink-Owner", h.owner)
	request.Header.Set("X-Pinelink-Device", h.device)
	request.Header.Set("X-Pinelink-Timestamp", strconv.FormatInt(time.Now().Unix(), 10))
	request.Header.Set("X-Pinelink-Signature", "invalid")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("tampered signature returned %s", response.Status)
	}
}
