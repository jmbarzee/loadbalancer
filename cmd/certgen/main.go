package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"

	"github.com/jmbarzee/loadbalancer/internal/cert"
)

func main() {
	err := generate()
	if err != nil {
		fmt.Errorf("Failed to generate certs")
		os.Exit(1)
	}
	fmt.Println("client and server cert generation succeeded")

	err = verify()
	if err != nil {
		fmt.Errorf("Failed to verify certs")
		os.Exit(1)
	}
	fmt.Println("client and server cert verification succeeded")
}

func generate() error {
	ca, err := cert.NewCertificateAuthority(cert.CaPubKeyFile, cert.CaPriKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read certificate authority: %w", err)
	}

	err = genAndSave(ca, cert.ClientPubKeyFile, cert.ClientPriKeyFile)
	if err != nil {
		return fmt.Errorf("failed to generate client certificate: %w", err)
	}

	err = genAndSave(ca, cert.ServerPubKeyFile, cert.ServerPriKeyFile)
	if err != nil {
		return fmt.Errorf("failed to generate server certificate: %w", err)
	}
	return nil
}

func verify() error {
	ca, err := cert.NewCertificateAuthority(cert.CaPubKeyFile, cert.CaPriKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read certificate authority: %w", err)
	}

	clientCert, err := cert.New(cert.ClientPubKeyFile, cert.ClientPriKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read client certificate: %w", err)
	}

	serverCert, err := cert.New(cert.ServerPubKeyFile, cert.ServerPriKeyFile)
	if err != nil {
		return fmt.Errorf("failed to read server certificate: %w", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(ca.Public)

	opts := x509.VerifyOptions{
		Roots:         roots,
		Intermediates: x509.NewCertPool(),
		DNSName:       cert.ServerName,
	}

	if _, err := clientCert.Public.Verify(opts); err != nil {
		return fmt.Errorf("failed to verify client certificate: %v", err)
	}

	if _, err := serverCert.Public.Verify(opts); err != nil {
		return fmt.Errorf("failed to verify server certificate: %v", err)
	}

	return nil
}

func genAndSave(ca *cert.KeyPair, publicKeyFile, privateKeyFile string) error {
	cert := &x509.Certificate{
		Subject: pkix.Name{
			Organization: []string{"teleport"},
			Country:      []string{"US"},
			Province:     []string{"Utah"},
			Locality:     []string{"Salt Lake City"},
			CommonName:   "localhost",
		},
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
		DNSNames:     []string{"localhost", cert.ServerName, "*." + cert.ServerName},
		IPAddresses:  []net.IP{net.IPv4(127, 0, 0, 1), net.IPv6loopback},

		NotBefore: time.Now(),
		NotAfter:  time.Now().AddDate(0, 1, 0),
		// Apple recently changed the model that if a self signed certificate is greater than `825`
		// golang did this as well

		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:    x509.KeyUsageDigitalSignature,

		SerialNumber: big.NewInt(1337),
	}

	certPrivKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return err
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, cert, ca.Public, &certPrivKey.PublicKey, ca.Private)
	if err != nil {
		return err
	}

	certPEM := new(bytes.Buffer)
	pem.Encode(certPEM, &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	certPrivKeyPEM := new(bytes.Buffer)
	pem.Encode(certPrivKeyPEM, &pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(certPrivKey),
	})

	if err := os.WriteFile(publicKeyFile, certPEM.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write to public key file: %w", err)
	}

	if err := os.WriteFile(privateKeyFile, certPrivKeyPEM.Bytes(), 0644); err != nil {
		return fmt.Errorf("failed to write to private key file: %w", err)
	}

	return nil
}
