// Package httptrust supplies current public HTTPS roots to the Win7 build.
package httptrust

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"errors"
	"fmt"
)

//go:embed cacert.pem
var publicRoots []byte

type blockedCertificates map[[sha256.Size]byte]bool

func certificateID(cert *x509.Certificate) [sha256.Size]byte {
	return sha256.Sum256(cert.Raw)
}

func rootPool(system []*x509.Certificate, blocked blockedCertificates) (*x509.CertPool, error) {
	pool := x509.NewCertPool()
	data := publicRoots
	count := 0
	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		data = rest
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("parse bundled HTTPS root: %w", err)
		}
		count++
		if !blocked[certificateID(cert)] {
			pool.AddCert(cert)
		}
	}
	if count == 0 {
		return nil, errors.New("bundled HTTPS roots are empty")
	}
	for _, cert := range system {
		if !blocked[certificateID(cert)] {
			pool.AddCert(cert)
		}
	}
	return pool, nil
}

func configureTLS(base *tls.Config, roots *x509.CertPool, blocked blockedCertificates) *tls.Config {
	config := &tls.Config{}
	if base != nil {
		config = base.Clone()
	}
	config.RootCAs = roots
	previous := config.VerifyConnection
	config.VerifyConnection = func(state tls.ConnectionState) error {
		// Keep explicit Windows distrust decisions, including a blocked leaf or
		// intermediate, when using Go's verifier instead of CryptoAPI.
		for _, cert := range state.PeerCertificates {
			if blocked[certificateID(cert)] {
				return errors.New("HTTPS certificate is explicitly distrusted by Windows")
			}
		}
		for _, chain := range state.VerifiedChains {
			for _, cert := range chain {
				if blocked[certificateID(cert)] {
					return errors.New("HTTPS certificate chain is explicitly distrusted by Windows")
				}
			}
		}
		if previous != nil {
			return previous(state)
		}
		return nil
	}
	return config
}
