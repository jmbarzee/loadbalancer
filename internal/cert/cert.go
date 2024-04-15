package cert

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path"
)

var (
	ServerName = "jmbarzee.com"

	certDir = "certs"

	CaPubKeyFile = path.Join(certDir, "ca.pem")
	CaPriKeyFile = path.Join(certDir, "ca.key")

	ClientPubKeyFile = path.Join(certDir, "client.pem")
	ClientPriKeyFile = path.Join(certDir, "client.key")

	ServerPubKeyFile = path.Join(certDir, "server.pem")
	ServerPriKeyFile = path.Join(certDir, "server.key")
)

type KeyPair struct {
	Public  *x509.Certificate
	Private *rsa.PrivateKey
}

func NewCertificateAuthority(publicKeyFile, privateKeyFile string) (*KeyPair, error) {
	pubKey, err := readPublicKey(publicKeyFile)
	if err != nil {
		return nil, err
	}

	priKey, err := readPrivatePKCS8(privateKeyFile)
	if err != nil {
		return nil, err
	}

	return &KeyPair{
		Public:  pubKey,
		Private: priKey,
	}, nil
}

func New(publicKeyFile, privateKeyFile string) (*KeyPair, error) {
	pubKey, err := readPublicKey(publicKeyFile)
	if err != nil {
		return nil, err
	}

	priKey, err := readPrivatePKCS1(privateKeyFile)
	if err != nil {
		return nil, err
	}

	return &KeyPair{
		Public:  pubKey,
		Private: priKey,
	}, nil

}

func readPublicKey(pubKeyFile string) (*x509.Certificate, error) {
	pubPemBlock, err := readAndPemDecode(pubKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read and decode: %w", err)
	}

	return x509.ParseCertificate(pubPemBlock.Bytes)
}

func readPrivatePKCS1(priKeyFile string) (*rsa.PrivateKey, error) {
	priPemBlock, err := readAndPemDecode(priKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read and decode: %w", err)
	}

	return x509.ParsePKCS1PrivateKey(priPemBlock.Bytes)
}

func readPrivatePKCS8(priKeyFile string) (*rsa.PrivateKey, error) {
	priPemBlock, err := readAndPemDecode(priKeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read and decode: %w", err)
	}

	priKey, err := x509.ParsePKCS8PrivateKey(priPemBlock.Bytes)
	return priKey.(*rsa.PrivateKey), err
}

func readAndPemDecode(keyFile string) (*pem.Block, error) {
	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, err
	}

	pemBlock, _ := pem.Decode(keyData)
	return pemBlock, nil
}
