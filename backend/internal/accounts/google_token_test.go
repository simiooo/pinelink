package accounts

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func TestGoogleVerifiesSignedIssuerAudienceNonceAndMailbox(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	claims := map[string]any{"iss": googleIssuer, "sub": "google-subject", "aud": "client", "azp": "client", "nonce": "expected", "email": "person@example.com", "email_verified": true, "iat": time.Now().Unix(), "exp": time.Now().Add(time.Hour).Unix()}
	encode := func(value any) string {
		raw, _ := json.Marshal(value)
		return base64.RawURLEncoding.EncodeToString(raw)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := encode(map[string]string{"alg": "RS256", "typ": "JWT"}) + "." + encode(claims)
		hash := sha256.Sum256([]byte(body))
		signature, e := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
		if e != nil {
			t.Error(e)
			w.WriteHeader(500)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture", "token_type": "Bearer", "id_token": body + "." + base64.RawURLEncoding.EncodeToString(signature)})
	}))
	defer server.Close()
	provider := &oidcGoogle{config: oauth2.Config{ClientID: "client", ClientSecret: "secret", RedirectURL: "http://localhost", Endpoint: oauth2.Endpoint{TokenURL: server.URL}}, verifier: oidc.NewVerifier(googleIssuer, &oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&key.PublicKey}}, &oidc.Config{ClientID: "client", SupportedSigningAlgs: []string{"RS256"}})}
	if _, err = provider.Exchange(t.Context(), "code", "verifier", "expected"); err != nil {
		t.Fatalf("valid signed token: %v", err)
	}
	for _, test := range []struct {
		field string
		value any
	}{{"iss", "https://attacker.example"}, {"aud", "other-client"}, {"nonce", "replayed"}, {"email_verified", false}, {"azp", "other-client"}, {"exp", time.Now().Add(-time.Hour).Unix()}} {
		t.Run(fmt.Sprint(test.field), func(t *testing.T) {
			old := claims[test.field]
			claims[test.field] = test.value
			defer func() { claims[test.field] = old }()
			if _, err = provider.Exchange(t.Context(), "code", "verifier", "expected"); err == nil {
				t.Fatal("invalid identity token accepted")
			}
		})
	}
}
