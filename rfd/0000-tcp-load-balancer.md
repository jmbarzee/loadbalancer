---
authors: Jacob Barzee jmbarzee@gmail.com
state: draft
---


# **RFD 0000 - TCP Load Balancer**

# Required Approvers
2 of 3 (@jakule, @smallinsky, @rosstimothy)


# What
A Layer 4 load balancer library with rate limiting and health checking. Server leveraging library behind mTLS.

# Why
Describe the features and design of a layer 4 load balancer as a catalyst for discussion and guide for work. Generally depict my skills as an engineer by simulating working with a small team from teleport engineering.

# Details
**Author’s Note:** Parts of this document cover “optional” items. These are included to depict the possible design choices that would be considered when designing a real-world system. These are not actually intended to be implemented within the scope of the interview.



## Scope
### Library
- Least connections connection forwarder – upstream connection tracking
- Per-client connection rate limiter – downstream connection tracking
- Health checking – only healthy upstreams should receive new connections
### Server
- Accept and forward connections to upstreams using the library
- Simple authorization scheme – what upstreams are available to which clients
- mTLS authentication – mutual identity authentication

## Security Considerations
Downstream clients should be considered untrustworthy until authenticated, no operation should proceed authentication.

Overloading Upstream systems should be prevented through rate limiting downstream clients. Downstream clients will receive errors when they are being rate limited. These errors and others shouldn't expose internal information about upstream hosts, inaccessible host groups, or other downstream clients. In general, errors should give ample information to a downstream client, but nothing more than necessary for them to address the issue.

### Remaining Risks (not exhaustive)
- Self signed certs are generally less secure and more easily imitated
- Sum of possible downstream client connections still is more load than upstream can handle.
- Unauthenticated downstreams overwhelm load balancer with requests to authenticate.
- Mistakes in implementation could offer a means to create zombie connections or goroutine leaks, eventually crashing the load balancer.

For further details see [Authentication & mTLS](###Authentication-&-mTLS)

## Implementation - Library
### Rate Limiting
Rate Limiting is performed for downstreams based on a count of existing connections. The count will be incremented by opening connections and decremented by closing ones. It will be critical that all possible forms of connections ending will decrement the counter, otherwise clients will be rate limited unnecessarily.

### Load Balancing
Load balancing is preformed for healthy upstreams. When a new connection is started, a priority queue will be used to pull the upstream with the least connections. The priority queue will be implemented with a golang heap with order based on the number of existing connections to an upstream. Unhealthy upstreams will be removed if they are first in the queue, and the next upstream will be checked.

### Health Checking
Health checking is both passive and active. Upstreams are healthy only after succeeding a routine (+ jitter) health check. Upstreams become unhealthy and thus unavailable after either failing a health check, timing out, or returning a connection error. Configuration for the number of retries and length of time outs to an upstream may be configurable at server startup, determined at implementation.

### API

```go
// NewLoadBalancer takes configuration values and creates a LoadBalancer with it
// Config could include rate limits, upstreams, timeouts, etc.
func NewLoadBalancer(cfg Config) ImplementedLoadBalancer {
    ...
}

type LoadBalancer interface {
    // Start spins up management goroutines for the LoadBalancer
    // error is nil only when LoadBalancer configuration is functional
    // otherwise error indicates what prevented start up
    Start(ctx context.Context) error

    // Stop prevents the LoadBalancer from taking any additional connections
    // but allows existing connections to close, so long as ctx hasn't ended.
    // error is nil only when all connections close gracefully
    // otherwise, error offers insight into why the connections couldn't close gracefully.
    Stop(ctx context.Context) error

    // Handle is the primary use of LoadBalancer
    // Handle requires that LoadBalancer.Start(ctx) error has been called
    // Handle first checks that the client shouldn't be rate limited
    // If the client is allowed to connect, 
    // LoadBalancer choses the upstream with the least connections 
    // and then spins up two goroutines to copy reads from either end
    // to the other. It also begins tracking the connection for future
    // rate limiting and possible closing if Stop is called.
    // If the client is rate limited, the connection will be reset 
    Handle(clientID string, upstreamGroup string, conn net.TCPConn)
}

// Upstream may look something like this.
// It maybe split into two types upon implementation.
// One for inside the library which supports storing
// the health state of Upstream.
// Another for inserting the Upstream into the LoadBalancer.
type Upstream interface {
    // ID is used primarily to look up the Upstream's connections
    // in the rate limit cache. Maybe better thought of as the "connection tracker".
    ID() string // possibly changed to a typed uuid

    // Provides necessary information to call net.DialTCP()
    TCPAddr() net.TCPAddr
    // Healthy returns weather or not the upstream is healthy
    // and can be passed new connections.
    Healthy() bool
}

// UpstreamGroups is used to look up upstreams
// Data is purely illustrative
var UpstreamGroups = map[string][]Upstream{
    "UIServers" : []Upstream{
        ...
    },
    "BackendServers" : []Upstream{
        ...
    },
    "SpecialPremiumCustomerServers" : []Upstream{
        ...
    },
}
```


### Internals
Leverage goroutines to build a “concurrent-first” design. This is a typical solution for golang and follows language paradigms, treating goroutines as lightweight tools for handling blocking operations. Two goroutines will be spun up for each connection and will call blocking reads for each end. When reads return, any data will be written to the other side. If one connection is closed, the other will be closed as well. A worker routine(s) will keep track of existing connections and routinely report existing connections to the rate limiting cache. This will enable tracking client connections for rate limiting and tracking server connections for load balancing.

![](0000-tcp-load-balancer/library-blocking-io.png)


This design, though non-traditional for industry standard load balancers, lets us avoid fiddling with net.TCPConn ReadDeadline lengths, which would likely be brittle. This solution might also be seen as a bit of a memory/complexity trade-off. Overhead for the connections takes non-trivial amounts of memory. 4k ram per routine. At least 2 routines per connection. ~1Mil connections = 8 Gb for goroutines alone, not counting any data being moved back and forth or any structures being leveraged in or by the net package.


## Implementation - Server

### CLI UX
A straight forward CLI will be offered, and will start the server based on hard coded configurations. The process will listen for signals and stop accordingly. If the process is sent a signal to exit, it will attempt to close gracefully. If it does successfully, it will return 0. Otherwise, it will return 1, the catchall for general errors. It may be worth differentiating between a failed start and a failed graceful exit, but for now, we'll leverage STDOUT and/or STDERR to inform the user what happened.

```go 
// Hardcoded values used for CLI
// Production quality applications would pull most of this from the env.
// the more complicated definitions could come from an endpoint or data lake
// and possibly be occasionally updated at runtime.
var (
    healthCheckInterval = time.Millisecond * 5000
    listeningPort = 8080
    listeningAddr = "" // Unset will allow the net package to listen on all IPs
    numWorkers = 1 // may be unused at implementation. 1 worker may be enough to keep track of all existing connections.


    // see UpstreamGroups

    // see ClientWhitelist
)
```

### Authentication & mTLS
The server will support TLS 1.3 (2018), which dropped support for older, less secure cryptographic features and it sped up TLS handshakes, among other improvements. The server will support at least one of the recommended suites; cipher suites supported by TLS 1.3:
- TLS_AES_128_GCM_SHA256 (0x13, 0x01)     >recommended<
- TLS_AES_256_GCM_SHA384 (0x13, 0x02)     >recommended<
- TLS_AES_128_CCM_SHA256 (0x13, 0x04)
- TLS_AES_128_CCM_8_SHA256 (0x13, 0x05)
- TLS_CHACHA20_POLY1305_SHA256            >recommended<

Certificates should typically be signed by a certificate authority, for the purposes of this challenge, they will be self-signed. This is generally considered less secure. Certificates can be created by a separate (included) utility and then saved to disk. The server will pick them up at start up, or exit if they are not found. Obviously, certificates will be prevented from being saved to disk with `.gitignore`. Not a production quality solution, but it will serve for the scope of the challenge.


### Authorization
The server will authorize new connections based on a simple scheme defining upstreamGroups and clients which are allowed to access them. It will be whitelist only. Clients will be identified by either the subject of the certificate, or just the CN (common name) from the subject. (chosen during implementation) By leveraging SNI, a TLS extension, A client's upstream will be chosen based on the server listed in client hello message.

```go
// ClientWhitelist is an explicit map of clientIDs to 
// available upstreamGroups (either the subject of the certificate,
// or just the CN (common name) from the subject)
var ClientWhitelist = map[string][]string{
    "FreeTrialClient": []string{
        "UIServers",
    },
    "StandardClient": []string{
        "UIServers",
        "BackendServers",
    },
    "SpecialPremiumClient": []string{
        "UIServers",
        "BackendServers",
        "SpecialPremiumCustomerServers",
    },
}
```
For an understanding of upstreamGroups see [API](###API)


![](0000-tcp-load-balancer/server.png)

## Additional considerations (optional)

### Upstream Connection Pools
Generally a good feature for any system which consistently holds many connections to another service/system. Pools could be dynamically sized to prevent resource overuse. Pools can also be created per worker to entirely avoid contention.

### Service Discovery
A common integration for load balancers, gateways, and proxy servers. Service discovery automates adopting and forgetting upstream hosts. This enables auto scaling and cooperates nicely with ephemeral services and systems.
