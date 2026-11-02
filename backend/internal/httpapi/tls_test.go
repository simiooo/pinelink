package httpapi

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestTLSProjectedSecretRotation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("atomic projected Secret symlinks require Linux")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "test CA"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatal(err)
	}
	directory := t.TempDir()
	for _, revision := range []int64{2, 3} {
		path := filepath.Join(directory, big.NewInt(revision).String())
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
		leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		leaf := &x509.Certificate{SerialNumber: big.NewInt(revision), NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, KeyUsage: x509.KeyUsageDigitalSignature}
		cert, err := x509.CreateCertificate(rand.Reader, leaf, ca, &leafKey.PublicKey, key)
		if err != nil {
			t.Fatal(err)
		}
		private, _ := x509.MarshalPKCS8PrivateKey(leafKey)
		if err := os.WriteFile(filepath.Join(path, "tls.crt"), pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), 0600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "tls.key"), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: private}), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink("2", filepath.Join(directory, "..data")); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tls.crt", "tls.key"} {
		if err := os.Symlink(filepath.Join("..data", name), filepath.Join(directory, name)); err != nil {
			t.Fatal(err)
		}
	}
	config, err := LoadTLSConfig(filepath.Join(directory, "tls.crt"), filepath.Join(directory, "tls.key"))
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) }))
	server.Listener = tls.NewListener(server.Listener, config)
	server.Start()
	defer server.Close()
	roots := x509.NewCertPool()
	roots.AddCert(ca)
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots}, DisableKeepAlives: true}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: time.Second}
	for _, serial := range []int64{2, 3} {
		if serial == 3 {
			if err := os.Symlink("3", filepath.Join(directory, "..next")); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(filepath.Join(directory, "..next"), filepath.Join(directory, "..data")); err != nil {
				t.Fatal(err)
			}
		}
		response, err := client.Get("https" + server.URL[len("http"):])
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.TLS.PeerCertificates[0].SerialNumber.Int64() != serial {
			t.Fatal("server retained an old Secret revision")
		}
	}
}

func TestTLSRequiresValidPairAtStartup(t *testing.T) {
	if _, err := LoadTLSConfig("", ""); err == nil {
		t.Fatal("accepted missing TLS configuration")
	}
	if _, err := LoadTLSConfig(filepath.Join(t.TempDir(), "missing.crt"), "missing.key"); err == nil {
		t.Fatal("accepted an unreadable pair")
	}
}
