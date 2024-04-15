package loadbalancer

import (
	"crypto/tls"
	"crypto/x509"
	"log"
	"net"
	"time"

	"github.com/google/uuid"
	"github.com/jmbarzee/loadbalancer/core"
)

var (
	defaultHealthCheckInterval = time.Second * 15

	defaultRetryAttempts        = 5
	defaultRetryBackoffInterval = time.Second * 5
	defaultRetryBackoffMax      = time.Minute * 5
)

var _ ConfigProvider = (*Config)(nil)

// Config is an implementation of ConfigProvider
type Config struct {
	healthCheckInterval time.Duration

	retryAttempts        int
	retryBackoffInterval time.Duration
	retryBackoffMax      time.Duration

	rootCAs *x509.CertPool
	certs   []tls.Certificate

	logger *log.Logger
}

func NewConfigDefault(
	rootCAs *x509.CertPool,
	certs []tls.Certificate,
	logger *log.Logger,
) Config {

	return Config{
		healthCheckInterval:  defaultHealthCheckInterval,
		retryAttempts:        defaultRetryAttempts,
		retryBackoffInterval: defaultRetryBackoffInterval,
		retryBackoffMax:      defaultRetryBackoffMax,
		rootCAs:              rootCAs,
		certs:                certs,
		logger:               logger,
	}
}

func NewConfig(
	healthCheckInterval time.Duration,
	retryAttempts int,
	retryBackoffInterval time.Duration,
	retryBackoffMax time.Duration,
	rootCAs *x509.CertPool,
	certs []tls.Certificate,
	logger *log.Logger,
) Config {
	return Config{
		healthCheckInterval:  healthCheckInterval,
		retryAttempts:        retryAttempts,
		retryBackoffInterval: retryBackoffInterval,
		retryBackoffMax:      retryBackoffMax,
		rootCAs:              rootCAs,
		certs:                certs,
		logger:               logger,
	}
}

func (c Config) HealthCheckInterval() time.Duration  { return c.healthCheckInterval }
func (c Config) RetryAttempts() int                  { return c.retryAttempts }
func (c Config) RetryBackoffInterval() time.Duration { return c.retryBackoffInterval }
func (c Config) RetryBackoffMax() time.Duration      { return c.retryBackoffMax }
func (c Config) RootCAs() *x509.CertPool             { return c.rootCAs }
func (c Config) Certs() []tls.Certificate            { return c.certs }
func (c Config) Logger() *log.Logger                 { return c.logger }

var _ core.Upstream = (*Upstream)(nil)

// Upstream is an implementation of core.Upstream
type Upstream struct {
	id      uuid.UUID
	group   string
	tcpAddr *net.TCPAddr
}

func NewUpstream(
	id uuid.UUID,
	group string,
	tcpAddr *net.TCPAddr,
) Upstream {
	return Upstream{
		id:      id,
		group:   group,
		tcpAddr: tcpAddr,
	}
}

// ID is used primarily to look up the Upstream's connections
// in the rate limit cache. Maybe better thought of as the "connection tracker".
func (u Upstream) ID() uuid.UUID { return u.id }

// Group returns the group name that the upstream
func (u Upstream) Group() string { return u.group }

// Provides necessary information to call net.DialTCP()
func (u Upstream) TCPAddr() *net.TCPAddr { return u.tcpAddr }

var _ core.Downstream = (*Downstream)(nil)

// Downstream is an implementation of core.Downstream
type Downstream struct {
	id                  string
	allowedServerGroups []string
	maxConnections      uint32
}

func NewDownstream(
	id string,
	allowedServerGroups []string,
	maxConnections uint32,
) Downstream {
	return Downstream{
		id:                  id,
		allowedServerGroups: allowedServerGroups,
		maxConnections:      maxConnections,
	}
}

// DownstreamID is the CN from the subject of the clients provided certificate.
func (d Downstream) ID() string { return d.id }

// AllowedServerGroups provides a slice of server groups which the downstream is allowed to connect to.
// Not used by core library, only used by github.com/jmbarzee/loadbalancer/
func (d Downstream) AllowedServerGroups() []string { return d.allowedServerGroups }

// MaxConnections is the number of connections which will be allowed by rate limiting
func (d Downstream) MaxConnections() uint32 { return d.maxConnections }
