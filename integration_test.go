package loadbalancer

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"log"
	"net"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jmbarzee/loadbalancer/core"
	"github.com/jmbarzee/loadbalancer/internal/cert"
)

func listenEcho(addr *net.TCPAddr) {
	listener, err := net.ListenTCP(addr.Network(), addr)
	if err != nil {
		// bad practice to call t.Fatal from non-test goroutine
		panic(err)
	}

	conn, err := listener.Accept()
	io.Copy(conn, conn)
	listener.Close()
}

func dial(t *testing.T, addr *net.TCPAddr) *tls.Conn {
	t.Helper()

	caCertPool, clientCerts := getCAsAndClientCerts(t)

	cfg := &tls.Config{
		ServerName:   cert.ServerName,
		RootCAs:      caCertPool,
		Certificates: clientCerts,
	}

	conn, err := tls.Dial(addr.Network(), addr.String(), cfg)
	if err != nil {
		t.Error(err)
	}
	return conn
}

func getCAsAndClientCerts(t *testing.T) (*x509.CertPool, []tls.Certificate) {
	t.Helper()

	serverCert, err := tls.LoadX509KeyPair(cert.ServerPubKeyFile, cert.ServerPriKeyFile)
	if err != nil {
		log.Fatal(err)
	}

	caCert, err := cert.NewCertificateAuthority(cert.CaPubKeyFile, cert.CaPriKeyFile)
	if err != nil {
		t.Error(err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(caCert.Public)

	return caCertPool, []tls.Certificate{serverCert}
}

func getCAsAndServerCerts(t *testing.T) (*x509.CertPool, []tls.Certificate) {
	t.Helper()

	clientCert, err := tls.LoadX509KeyPair(cert.ClientPubKeyFile, cert.ClientPriKeyFile)
	if err != nil {
		log.Fatal(err)
	}

	caCert, err := cert.NewCertificateAuthority(cert.CaPubKeyFile, cert.CaPriKeyFile)
	if err != nil {
		t.Error(err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AddCert(caCert.Public)

	return caCertPool, []tls.Certificate{clientCert}
}
func TestIntegration(t *testing.T) {

	selfIP := net.ParseIP("127.0.0.1")

	upPort := 9090
	upAddr := &net.TCPAddr{IP: selfIP, Port: upPort}

	lbPort := 1443
	lbAddr := &net.TCPAddr{IP: selfIP, Port: lbPort}

	upID := uuid.New()

	ups := []core.Upstream{
		NewUpstream(upID, cert.ServerName, upAddr),
	}

	downs := []core.Downstream{
		NewDownstream("localhost", []string{cert.ServerName}, 10),
	}

	logBuf := &bytes.Buffer{}
	logger := log.New(logBuf, "", log.LstdFlags)

	caCertPool, certs := getCAsAndServerCerts(t)
	cfg := NewConfigDefault(caCertPool, certs, logger)

	go listenEcho(upAddr)

	ctx := context.Background()
	s := NewServer(cfg, downs, ups)
	go func() {
		err := s.Listen(ctx, lbAddr)
		if err != nil {
			// bad practice to call t.Fatal from non-test goroutine
			panic(err)
		}
	}()

	conn := dial(t, lbAddr)

	testData := []byte("This data should be echoed back")
	n, err := conn.Write(testData)
	if n != len(testData) {
		t.Errorf("failed to write all bytes to conn")
	}
	if err != nil {
		t.Errorf("got error while writing to conn: %v", err)
	}

	recvBuff := make([]byte, len(testData))
	n, err = conn.Read(recvBuff)
	if n != len(testData) {
		t.Errorf("failed to read all bytes from conn")
	}
	if err != nil {
		t.Errorf("got error while reading from conn: %v", err)
	}
	if !reflect.DeepEqual(testData, recvBuff) {
		t.Errorf("bytes passed through did not match")
	}
}
