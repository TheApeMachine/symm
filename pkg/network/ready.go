package network

import "context"

// ReadyTransport is implemented by transports that have a blocking initialization phase.
type ReadyTransport interface {
	Ready(ctx context.Context) error
}
