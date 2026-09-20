package network

import "io"

// ManagedTransport is an io.ReadWriteCloser that also provides traits and status.
type ManagedTransport interface {
	io.ReadWriteCloser
	Traits() TransportTraits
	Status() TransportStatus
}
