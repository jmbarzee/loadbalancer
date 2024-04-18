package loadbalancer

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net"

	"github.com/jmbarzee/loadbalancer/core"
)

// Server
type Server struct {
	// Trafficker handles incoming connections after they have been authenticated
	trafficker core.Trafficker

	downstreamAuthz map[string]core.Downstream

	logger *log.Logger

	config ConfigProvider
}

// ConfigProvider offers the values necessary to create a new loadbalancer.Server
type ConfigProvider interface {
	core.Config

	RootCAs() *x509.CertPool
	Certs() []tls.Certificate
	Logger() *log.Logger
}

// NewServer returns a new layer 4 load balancing server
func NewServer(cfg ConfigProvider, downs []core.Downstream, ups []core.Upstream) *Server {
	logger := cfg.Logger()
	authz := make(map[string]core.Downstream, len(downs))
	for _, down := range downs {
		authz[down.ID()] = down
	}

	return &Server{
		trafficker:      *core.NewTrafficker(cfg, downs, ups, logger),
		downstreamAuthz: authz,
		logger:          logger,
		config:          cfg,
	}
}

// Start initializes a series of underlying goroutines
// addr is passed directly to net.ListenTCP
func (s *Server) Listen(ctx context.Context, addr *net.TCPAddr) error {
	err := s.trafficker.Start(ctx)
	if err != nil {
		// TODO, wrap error for clarity?
		return err
	}

	listener, err := net.ListenTCP(addr.Network(), addr)
	if err != nil {
		// TODO, wrap error for clarity?
		return err
	}

	// TODO tcp listener configuration happens here
	defer listener.Close() // TODO we could handle/log this error

	for {
		conn, err := listener.Accept()
		if err != nil {
			// TODO, handle error. Can we receive errors that should end the listening?
			continue
		}
		go s.handle(conn)
	}
}

func (s *Server) handle(conn net.Conn) {
	var hello *tls.ClientHelloInfo

	tlsConfig := s.getTLSConfig()

	tlsConfig.GetConfigForClient = func(argHello *tls.ClientHelloInfo) (*tls.Config, error) {
		hello = new(tls.ClientHelloInfo)
		*hello = *argHello
		return nil, nil
	}

	connTLS := tls.Server(conn, tlsConfig)

	err := connTLS.Handshake()
	if err != nil {
		s.logger.Printf("Failed to preform handshake: %v", err)
	}

	requestedUpstream := hello.ServerName

	// TODO should we handle multiple certificates?
	downstreamCert := connTLS.ConnectionState().PeerCertificates[0]
	downstreamID := downstreamCert.Subject.CommonName

	if err := s.downstreamAuthorized(downstreamID, requestedUpstream); err != nil {
		s.logger.Printf("Error during authorization: %v", err)
		return
	}

	s.trafficker.Handle(downstreamID, requestedUpstream, connTLS.NetConn())
}

func (s *Server) getTLSConfig() *tls.Config {
	return &tls.Config{
		ClientCAs:    s.config.RootCAs(),
		Certificates: s.config.Certs(),
		// Require client certificates (or VerifyConnection will run anyway and
		// panic accessing cs.PeerCertificates[0]) but don't verify them with the
		// default verifier. This will not disable VerifyConnection.
		ClientAuth: tls.RequireAndVerifyClientCert,
		VerifyConnection: func(cs tls.ConnectionState) error {
			opts := x509.VerifyOptions{
				DNSName:       cs.ServerName,
				Intermediates: x509.NewCertPool(),
				KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
			}
			for _, cert := range cs.PeerCertificates[1:] {
				opts.Intermediates.AddCert(cert)
			}
			_, err := cs.PeerCertificates[0].Verify(opts)
			return err
		},
	}
}

func (s *Server) downstreamAuthorized(downstreamID, upstreamGroup string) error {
	downstream, ok := s.downstreamAuthz[downstreamID]
	if !ok {
		return fmt.Errorf("downstream(%v) not found", downstreamID)
	}

	if !foundIn(upstreamGroup, downstream.AllowedServerGroups()) {
		return fmt.Errorf("Authz: downstream(%v) attempted to access unauthorized upstream(%v)\n", downstreamID, upstreamGroup)
	}
	return nil
}

func foundIn(target string, ss []string) bool {
	for _, s := range ss {
		if s == target {
			return true
		}
	}
	return false
}
