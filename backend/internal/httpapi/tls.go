package httpapi

import (
	"crypto/tls"
	"fmt"
	"path/filepath"
)

// LoadTLSConfig reads a projected Secret afresh on each handshake. Resolving
// ..data once keeps certificate and key in the same atomic Kubernetes revision.
func LoadTLSConfig(certFile, keyFile string) (*tls.Config, error) {
	load := func() (*tls.Certificate, error) {
		cert, key := certFile, keyFile
		if filepath.Dir(certFile) == filepath.Dir(keyFile) {
			if directory, err := filepath.EvalSymlinks(filepath.Join(filepath.Dir(certFile), "..data")); err == nil {
				cert, key = filepath.Join(directory, filepath.Base(certFile)), filepath.Join(directory, filepath.Base(keyFile))
			}
		}
		pair, err := tls.LoadX509KeyPair(cert, key)
		return &pair, err
	}
	if certFile == "" || keyFile == "" {
		return nil, fmt.Errorf("both HTTP TLS files are required")
	}
	if _, err := load(); err != nil {
		return nil, fmt.Errorf("load HTTP TLS certificate: %w", err)
	}
	return &tls.Config{MinVersion: tls.VersionTLS12, GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) { return load() }}, nil
}
