package secure

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"filippo.io/age"
	"github.com/minio/sio"
)

const Suite = "DARE20-AGE-X25519-v1"

type Identity struct {
	private *age.X25519Identity
	signing ed25519.PrivateKey
}

func LoadIdentity(ownerID, deviceID string) (*Identity, error) {
	directory, err := configDirectory()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return nil, err
	}
	digest := sha256.Sum256([]byte(ownerID + "\x00" + deviceID))
	prefix := filepath.Join(directory, hex.EncodeToString(digest[:16]))
	signing, err := loadSigningKey(prefix + ".sign")
	if err != nil {
		return nil, err
	}
	content, err := os.ReadFile(prefix + ".age")
	if err == nil {
		private, err := age.ParseX25519Identity(string(bytes.TrimSpace(content)))
		if err != nil {
			return nil, fmt.Errorf("parse device identity: %w", err)
		}
		return &Identity{private: private, signing: signing}, nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	private, err := age.GenerateX25519Identity()
	if err != nil {
		return nil, err
	}
	content, err = publishIdentityKey(prefix+".age", []byte(private.String()+"\n"))
	if err != nil {
		return nil, err
	}
	private, err = age.ParseX25519Identity(string(bytes.TrimSpace(content)))
	if err != nil {
		return nil, fmt.Errorf("parse device identity: %w", err)
	}
	return &Identity{private: private, signing: signing}, nil
}

func loadSigningKey(path string) (ed25519.PrivateKey, error) {
	content, err := os.ReadFile(path)
	if err == nil {
		decoded, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(content)))
		if err != nil || len(decoded) != ed25519.PrivateKeySize {
			return nil, fmt.Errorf("invalid device signing key")
		}
		return ed25519.PrivateKey(decoded), nil
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	_, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	content, err = publishIdentityKey(path, []byte(base64.StdEncoding.EncodeToString(private)+"\n"))
	if err != nil {
		return nil, err
	}
	decoded, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(content)))
	if err != nil || len(decoded) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf("invalid device signing key")
	}
	return ed25519.PrivateKey(decoded), nil
}

// Publish a complete key without replacing an identity another process created.
// The local receiver and foreground CLI may start for the first time together.
func publishIdentityKey(path string, content []byte) ([]byte, error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".identity-*")
	if err != nil {
		return nil, err
	}
	defer os.Remove(file.Name())
	if _, err = file.Write(content); err == nil {
		err = file.Sync()
	}
	closed := file.Close()
	if err != nil {
		return nil, err
	}
	if closed != nil {
		return nil, closed
	}
	if err = os.Link(file.Name(), path); err != nil && !os.IsExist(err) {
		return nil, err
	}
	if directory, e := os.Open(filepath.Dir(path)); e == nil {
		_ = directory.Sync()
		_ = directory.Close()
	}
	return os.ReadFile(path)
}

func (i *Identity) PublicKey() string {
	return i.private.Recipient().String()
}

func (i *Identity) SigningPublicKey() string {
	public := i.signing.Public().(ed25519.PublicKey)
	return base64.StdEncoding.EncodeToString(public)
}

func (i *Identity) Sign(message []byte) string {
	return base64.StdEncoding.EncodeToString(ed25519.Sign(i.signing, message))
}

type Manifest struct {
	Suite         string `json:"suite"`
	DataKey       []byte `json:"data_key"`
	PlaintextRoot string `json:"plaintext_root"`
	PlaintextSize int64  `json:"plaintext_size"`
	FileName      string `json:"file_name"`
}

func EncryptFile(input, output, plaintextRoot string, plaintextSize int64, fileName string, identity *Identity) ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	source, err := os.Open(input)
	if err != nil {
		return nil, err
	}
	defer source.Close()
	destination, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, err
	}
	if _, err := sio.Encrypt(destination, source, sio.Config{Key: key, MinVersion: sio.Version20, MaxVersion: sio.Version20}); err != nil {
		_ = destination.Close()
		return nil, err
	}
	if err := destination.Close(); err != nil {
		return nil, err
	}
	manifest := Manifest{Suite: Suite, DataKey: key, PlaintextRoot: plaintextRoot, PlaintextSize: plaintextSize, FileName: fileName}
	return SealManifest(manifest, []string{identity.PublicKey()})
}

func EncryptManifestStream(source io.Reader, destination io.Writer, root string, size int64, name string, identity *Identity) ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if _, err := sio.Encrypt(destination, source, sio.Config{Key: key, MinVersion: sio.Version20, MaxVersion: sio.Version20}); err != nil {
		return nil, err
	}
	return SealManifest(Manifest{Suite: Suite, DataKey: key, PlaintextRoot: root, PlaintextSize: size, FileName: name}, []string{identity.PublicKey()})
}

func SealManifest(manifest Manifest, publicKeys []string) ([]byte, error) {
	if len(manifest.DataKey) != 32 || manifest.Suite != Suite {
		return nil, fmt.Errorf("invalid plaintext manifest")
	}
	recipients := make([]age.Recipient, 0, len(publicKeys))
	for _, publicKey := range publicKeys {
		recipient, err := age.ParseX25519Recipient(publicKey)
		if err != nil {
			return nil, err
		}
		recipients = append(recipients, recipient)
	}
	var destination bytes.Buffer
	writer, err := age.Encrypt(&destination, recipients...)
	if err != nil {
		return nil, err
	}
	if err := json.NewEncoder(writer).Encode(manifest); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return destination.Bytes(), nil
}

func OpenManifest(ciphertext []byte, identity *Identity) (Manifest, error) {
	reader, err := age.Decrypt(bytes.NewReader(ciphertext), identity.private)
	if err != nil {
		return Manifest{}, err
	}
	var manifest Manifest
	if err := json.NewDecoder(reader).Decode(&manifest); err != nil {
		return Manifest{}, err
	}
	if manifest.Suite != Suite || len(manifest.DataKey) != 32 {
		return Manifest{}, fmt.Errorf("unsupported manifest")
	}
	return manifest, nil
}

func DecryptFile(input, output string, key []byte) error {
	source, err := os.Open(input)
	if err != nil {
		return err
	}
	defer source.Close()
	destination, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := sio.Decrypt(destination, source, sio.Config{Key: key, MinVersion: sio.Version20, MaxVersion: sio.Version20}); err != nil {
		_ = destination.Close()
		return err
	}
	return destination.Close()
}

// SIO encodes an empty source as zero bytes and its generic decryptor rejects
// that stream. Accept that one case only when the authenticated manifest
// explicitly commits to an empty plaintext. Key-only decryption stays strict.
func DecryptManifestFile(input, output string, manifest Manifest) error {
	info, err := os.Stat(input)
	if err != nil {
		return err
	}
	const emptyRoot = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if info.Size() == 0 && manifest.Suite == Suite && len(manifest.DataKey) == 32 && manifest.PlaintextSize == 0 && manifest.PlaintextRoot == emptyRoot {
		file, err := os.OpenFile(output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		if err = file.Sync(); err != nil {
			file.Close()
			return err
		}
		return file.Close()
	}
	return DecryptFile(input, output, manifest.DataKey)
}

// The manifest must already have been opened with this recipient's age key.
// Callers additionally verify the plaintext size and root before committing.
func DecryptManifestStream(source io.Reader, destination io.Writer, manifest Manifest) error {
	reader := bufio.NewReader(source)
	const emptyRoot = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if _, err := reader.Peek(1); err == io.EOF && manifest.Suite == Suite && len(manifest.DataKey) == 32 && manifest.PlaintextSize == 0 && manifest.PlaintextRoot == emptyRoot {
		return nil
	}
	_, err := sio.Decrypt(destination, reader, sio.Config{Key: manifest.DataKey, MinVersion: sio.Version20, MaxVersion: sio.Version20})
	return err
}

func SpoolPaths(dedupKey string, namespace ...string) (string, string, error) {
	directory, err := configDirectory()
	if err != nil {
		return "", "", err
	}
	directory = filepath.Join(directory, "uploads")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", "", err
	}
	cacheKey := dedupKey[:32]
	if len(namespace) > 0 {
		encoded, err := json.Marshal(append(namespace, dedupKey))
		if err != nil {
			return "", "", err
		}
		digest := sha256.Sum256(encoded)
		cacheKey = fmt.Sprintf("%x", digest)
	}
	prefix := filepath.Join(directory, cacheKey)
	return prefix + ".dare", prefix + ".age", nil
}

func configDirectory() (string, error) {
	if directory := os.Getenv("PINELINK_CONFIG_HOME"); directory != "" {
		return directory, nil
	}
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(directory, "pinelink"), nil
}

// Adopt a legacy cache only after authenticating its envelope for this device
// and verifying its plaintext identity. Leave all legacy files untouched.
func AdoptLegacySpool(key, ciphertextPath, envelopePath string, identity *Identity, root string, size int64) error {
	if _, err := os.Stat(ciphertextPath); err == nil {
		return nil
	}
	legacy, oldEnvelope, err := SpoolPaths(key)
	if err != nil {
		return err
	}
	envelope, err := os.ReadFile(oldEnvelope)
	if err != nil {
		return nil
	}
	manifest, err := OpenManifest(envelope, identity)
	if err != nil || manifest.PlaintextRoot != root || manifest.PlaintextSize != size {
		return nil
	}
	input, err := os.Open(legacy)
	if err != nil {
		return nil
	}
	defer input.Close()
	output, err := os.OpenFile(ciphertextPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(output, input)
	if copyErr == nil {
		copyErr = output.Sync()
	}
	closeErr := output.Close()
	if copyErr != nil {
		os.Remove(ciphertextPath)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(ciphertextPath)
		return closeErr
	}
	if err = os.WriteFile(envelopePath, envelope, 0o600); err != nil {
		os.Remove(ciphertextPath)
	}
	return err
}
