package rpcplugin

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"time"
)

// generateCertificate generates a temporary certificate for plugin
// authentication.
func generateCertificate(ctx context.Context, host string) (tls.Certificate, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return tls.Certificate{}, err
	}

	serialNumberLimit := new(big.Int).Lsh(big.NewInt(1), 128)
	sn, err := rand.Int(rand.Reader, serialNumberLimit)
	if err != nil {
		return tls.Certificate{}, err
	}

	template := &x509.Certificate{
		Subject: pkix.Name{
			CommonName:   host,
			Organization: []string{"rpcplugin"},
		},
		DNSNames: []string{host},
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageClientAuth,
			x509.ExtKeyUsageServerAuth,
		},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment | x509.KeyUsageKeyAgreement | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
		SerialNumber:          sn,
		NotBefore:             time.Now().Add(-30 * time.Second),
		NotAfter:              time.Now().Add(262980 * time.Hour),
		IsCA:                  true,
	}

	der, err := x509.CreateCertificate(rand.Reader, template, template, key.Public(), key)
	if err != nil {
		return tls.Certificate{}, err
	}

	var certOut bytes.Buffer
	if err := pem.Encode(&certOut, &pem.Block{Type: "CERTIFICATE", Bytes: der}); err != nil {
		return tls.Certificate{}, err
	}

	keyBytes := x509.MarshalPKCS1PrivateKey(key)

	var keyOut bytes.Buffer
	if err := pem.Encode(&keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return tls.Certificate{}, err
	}

	certPEM := certOut.Bytes()
	privateKeyPEM := keyOut.Bytes()

	cert, err := tls.X509KeyPair(certPEM, privateKeyPEM)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate temporary X509 keypair: %s", err)
	}

	return cert, nil
}

func decodeRawBase64Cert(src string) (*x509.Certificate, error) {
	asn1, err := base64.StdEncoding.DecodeString(src)
	if err != nil {
		// We'll also try RawStdEncoding, because that's what HashiCorp's
		// go-plugin uses and so this is more compatible. Support for
		// RawStdEncoding is not a required part of rpcplugin, so not all
		// implementations will support it.
		asn1, err = base64.RawStdEncoding.DecodeString(src)
		if err != nil {
			return nil, fmt.Errorf("failed to parse plugin server's temporary certificate: %s", err)
		}
	}

	return x509.ParseCertificate([]byte(asn1))
}
