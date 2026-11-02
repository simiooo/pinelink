package secure

import (
	"bytes"
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestConcurrentFirstIdentityDoesNotRotateKeys(t *testing.T) {
	t.Setenv("PINELINK_CONFIG_HOME", t.TempDir())
	const count = 16
	identities := make([]*Identity, count)
	errors := make([]error, count)
	var group sync.WaitGroup
	for index := range identities {
		group.Go(func() { identities[index], errors[index] = LoadIdentity("same-owner", "same-device") })
	}
	group.Wait()
	for index, identity := range identities {
		if errors[index] != nil {
			t.Fatal(errors[index])
		}
		if identity.LocalID() != identities[0].LocalID() || identity.PublicKey() != identities[0].PublicKey() {
			t.Fatal("concurrent startup created different device keys")
		}
	}
}

func TestAuthenticatedEmptyFile(t *testing.T) {
	t.Setenv("PINELINK_CONFIG_HOME", t.TempDir())
	identity, err := LoadIdentity("owner", "device")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "empty")
	ciphertext := filepath.Join(dir, "empty.dare")
	output := filepath.Join(dir, "decrypted")
	if err = os.WriteFile(input, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	envelope, err := EncryptFile(input, ciphertext, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", 0, "empty", identity)
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := OpenManifest(envelope, identity)
	if err != nil {
		t.Fatal(err)
	}
	if err = DecryptManifestFile(ciphertext, output, manifest); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(output); err != nil || info.Size() != 0 {
		t.Fatal("empty file did not roundtrip", err)
	}
	if err = DecryptFile(ciphertext, filepath.Join(dir, "key-only"), manifest.DataKey); err == nil {
		t.Fatal("key-only decrypt accepted an unauthenticated empty stream")
	}
	manifest.PlaintextSize = 1
	if err = DecryptManifestFile(ciphertext, filepath.Join(dir, "truncated"), manifest); err == nil {
		t.Fatal("empty ciphertext replaced a nonempty manifest")
	}
}

func TestSpoolNamespaceAndLegacyOwnership(t *testing.T) {
	t.Setenv("PINELINK_CONFIG_HOME", t.TempDir())
	owner, err := LoadIdentity("owner", "device")
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := LoadIdentity("outsider", "device")
	if err != nil {
		t.Fatal(err)
	}
	const key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	original, oldEnvelope, err := SpoolPaths(key)
	if err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(t.TempDir(), "input")
	if err = os.WriteFile(input, []byte("payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	envelope, err := EncryptFile(input, original, "plaintext-root", 7, "input", owner)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(oldEnvelope, envelope, 0o600); err != nil {
		t.Fatal(err)
	}
	mine, mineEnvelope, err := SpoolPaths(key, "server", "owner", "device", "scope")
	if err != nil {
		t.Fatal(err)
	}
	theirs, theirEnvelope, err := SpoolPaths(key, "server", "outsider", "device", "scope")
	if err != nil {
		t.Fatal(err)
	}
	if mine == theirs || mine == original {
		t.Fatal("spool namespace collided")
	}
	if err = AdoptLegacySpool(key, theirs, theirEnvelope, outsider, "plaintext-root", 7); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(theirs); !os.IsNotExist(err) {
		t.Fatal("another identity adopted a legacy spool")
	}
	if err = AdoptLegacySpool(key, mine, mineEnvelope, owner, "plaintext-root", 7); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(mine); err != nil {
		t.Fatal("authenticated legacy resume was lost", err)
	}
	if _, err = os.Stat(original); err != nil {
		t.Fatal("legacy source was changed")
	}
}

func TestEncryptedFileAndIdentity(t *testing.T) {
	t.Setenv("PINELINK_CONFIG_HOME", t.TempDir())
	identity, err := LoadIdentity("owner", "device")
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("secret file contents that must never be stored in plaintext")
	root := "9b3d7d8a6f6e5f4b3f24f77f3d0b55cb0a1a7e45a35a25c7e2a31ef08d51c4a1f2"
	directory := t.TempDir()
	input := filepath.Join(directory, "input.txt")
	ciphertext := filepath.Join(directory, "ciphertext.dare")
	output := filepath.Join(directory, "output.txt")
	if err := os.WriteFile(input, plaintext, 0o600); err != nil {
		t.Fatal(err)
	}
	envelope, err := EncryptFile(input, ciphertext, root, int64(len(plaintext)), "input.txt", identity)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := os.ReadFile(ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(stored, plaintext) || bytes.Contains(stored, plaintext) {
		t.Fatal("ciphertext contains plaintext")
	}
	manifest, err := OpenManifest(envelope, identity)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.PlaintextRoot != root {
		t.Fatal("manifest root mismatch")
	}
	if err := DecryptFile(ciphertext, output, manifest.DataKey); err != nil {
		t.Fatal(err)
	}
	decrypted, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(decrypted, plaintext) {
		t.Fatal("decrypted content differs")
	}
	message := []byte("signed request")
	signature, err := base64.StdEncoding.DecodeString(identity.Sign(message))
	if err != nil {
		t.Fatal(err)
	}
	public, err := base64.StdEncoding.DecodeString(identity.SigningPublicKey())
	if err != nil {
		t.Fatal(err)
	}
	if !ed25519.Verify(ed25519.PublicKey(public), message, signature) {
		t.Fatal("device signature did not verify")
	}
}
