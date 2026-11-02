package secure

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"math/big"
	"time"
)

func (i *Identity) LocalID() string {
	sum := sha256.Sum256(i.signing.Public().(ed25519.PublicKey))
	return hex.EncodeToString(sum[:])
}

// The device key is the peer's trust anchor. Local connections pin this public
// key directly, so they do not need a CA, DNS or a synchronized peer clock.
func (i *Identity) LocalCertificate() (tls.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return tls.Certificate{}, err
	}
	template := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: "Pinelink local device"}, NotBefore: time.Now().Add(-24 * time.Hour), NotAfter: time.Now().AddDate(1, 0, 0),
		KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, i.signing.Public(), i.signing)
	if err != nil {
		return tls.Certificate{}, err
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: i.signing}, nil
}

func ConfigDirectory() (string, error) { return configDirectory() }
