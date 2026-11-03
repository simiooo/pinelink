package accounts

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
	"golang.org/x/net/idna"
	"golang.org/x/text/unicode/norm"
)

func randomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func hash(value string) []byte { d := sha256.Sum256([]byte(value)); return d[:] }
func (s *Service) mac(domain, value string) []byte {
	h := hmac.New(sha256.New, s.key)
	h.Write([]byte(domain + "\n" + value))
	return h.Sum(nil)
}
func (s *Service) seal(plain []byte) []byte {
	block, _ := aes.NewCipher(s.key)
	gcm, _ := cipher.NewGCM(block)
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		panic(err)
	}
	return gcm.Seal(nonce, nonce, plain, []byte("pinelink-auth-v1"))
}
func (s *Service) open(sealed []byte) ([]byte, error) {
	block, _ := aes.NewCipher(s.key)
	gcm, _ := cipher.NewGCM(block)
	if len(sealed) < gcm.NonceSize() {
		return nil, denied()
	}
	return gcm.Open(nil, sealed[:gcm.NonceSize()], sealed[gcm.NonceSize():], []byte("pinelink-auth-v1"))
}

func normalizeEmail(value string) (string, error) {
	if len(value) > 320 || strings.TrimSpace(value) != value {
		return "", fault(400, "INVALID_EMAIL")
	}
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Name != "" || parsed.Address != value {
		return "", fault(400, "INVALID_EMAIL")
	}
	at := strings.LastIndexByte(value, '@')
	if at < 1 {
		return "", fault(400, "INVALID_EMAIL")
	}
	domain, err := idna.Lookup.ToASCII(value[at+1:])
	if err != nil || !strings.Contains(domain, ".") {
		return "", fault(400, "INVALID_EMAIL")
	}
	email := strings.ToLower(norm.NFC.String(value[:at]) + "@" + domain)
	if len(email) > 254 {
		return "", fault(400, "INVALID_EMAIL")
	}
	return email, nil
}

func passwordValue(password string) (string, error) {
	if !utf8.ValidString(password) || len(password) > 512 {
		return "", fault(400, "INVALID_PASSWORD")
	}
	password = norm.NFC.String(password)
	n := utf8.RuneCountInString(password)
	if n < 15 || n > 128 {
		return "", fault(400, "PASSWORD_LENGTH")
	}
	lower := strings.ToLower(password)
	common := []string{"123456789012345", "1234567890123456", "passwordpassword", "password123456789", "thisisapassword", "qwertyuiopasdfgh", "iloveyouiloveyou", "letmeinletmeinletmein"}
	for _, blocked := range common {
		if lower == blocked {
			return "", fault(400, "PASSWORD_COMMON")
		}
	}
	if strings.Contains(lower, "pinelink") {
		return "", fault(400, "PASSWORD_COMMON")
	}
	runes := []rune(password)
	same := true
	for _, r := range runes {
		if r != runes[0] {
			same = false
			break
		}
	}
	if same {
		return "", fault(400, "PASSWORD_COMMON")
	}
	return password, nil
}

func (s *Service) kdf(ctx context.Context, run func() string) (string, error) {
	select {
	case s.kdfSlots <- struct{}{}:
		defer func() { <-s.kdfSlots }()
		return run(), nil
	case <-ctx.Done():
		return "", fault(503, "AUTH_BUSY")
	default:
		return "", fault(429, "AUTH_BUSY")
	}
}
func (s *Service) passwordHash(ctx context.Context, password string) (string, error) {
	value, err := passwordValue(password)
	if err != nil {
		return "", err
	}
	if err = s.checkBreached(ctx, value); err != nil {
		return "", err
	}
	return s.kdf(ctx, func() string {
		salt := make([]byte, 16)
		rand.Read(salt)
		key := argon2.IDKey([]byte(value), salt, 3, 32*1024, 1, 32)
		return fmt.Sprintf("$argon2id$v=19$m=32768,t=3,p=1$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
	})
}
func (s *Service) passwordVerify(ctx context.Context, password, phc string) (bool, error) {
	if !utf8.ValidString(password) || len(password) > 512 {
		return false, nil
	}
	password = norm.NFC.String(password)
	parts := strings.Split(phc, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, denied()
	}
	params := strings.Split(parts[3], ",")
	if len(params) != 3 {
		return false, denied()
	}
	vals := make([]int, 3)
	for i, prefix := range []string{"m=", "t=", "p="} {
		if !strings.HasPrefix(params[i], prefix) {
			return false, denied()
		}
		v, err := strconv.Atoi(strings.TrimPrefix(params[i], prefix))
		if err != nil {
			return false, denied()
		}
		vals[i] = v
	}
	if vals[0] < 19456 || vals[0] > 65536 || vals[1] < 2 || vals[1] > 4 || vals[2] != 1 {
		return false, denied()
	}
	salt, e1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, e2 := base64.RawStdEncoding.DecodeString(parts[5])
	if e1 != nil || e2 != nil || len(salt) != 16 || len(want) != 32 {
		return false, denied()
	}
	got, err := s.kdf(ctx, func() string {
		return string(argon2.IDKey([]byte(password), salt, uint32(vals[1]), uint32(vals[0]), 1, 32))
	})
	return subtle.ConstantTimeCompare([]byte(got), want) == 1, err
}
