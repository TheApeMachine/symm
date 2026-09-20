package network

// TransportFailureMode represents the reason a transport failed.
type TransportFailureMode string

const (
	TransportFailureNone     TransportFailureMode = "none"
	TransportFailureNotReady TransportFailureMode = "not_ready"
	TransportFailureIO       TransportFailureMode = "io"
	TransportFailureTimeout  TransportFailureMode = "timeout"
)

// CircuitBreakerState represents the state of the circuit breaker for a transport.
type CircuitBreakerState int

const (
	CircuitClosed CircuitBreakerState = iota
	CircuitOpen
	CircuitHalfOpen
)

// TransportTraits describes the capabilities and guarantees of a transport.
type TransportTraits struct {
	Reliable  bool
	Ordered   bool
	Multicast bool
}

// TransportStatus represents the health and state of a transport.
type TransportStatus struct {
	LastFailureMode TransportFailureMode
	LastFailure     error
	SystemicFailure bool
	Degraded        bool
	Breaker         CircuitBreakerState
}
