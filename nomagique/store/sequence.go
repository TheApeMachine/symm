package store

import (
	"bytes"
	"context"

	"github.com/theapemachine/errnie"
)

/* SequenceServer retains immutable values at stable, append-only positions. */
type SequenceServer struct {
	values [][]byte
	index  uint64
}

/* NewSequence creates an empty sequence; the caller owns its lifetime. */
func NewSequence() *SequenceServer {
	return &SequenceServer{}
}

/* Write appends arrivals atomically and selects the graph's requested position. */
func (server *SequenceServer) Write(ctx context.Context, call Sequence_write) error {
	arrivals, err := call.Args().Append()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "sequence: read arrivals", err))
	}
	pending := make([][]byte, 0, arrivals.Len())

	for position := range arrivals.Len() {
		value, err := arrivals.At(position)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "sequence: read value", err))
		}

		if len(value) > 0 {
			pending = append(pending, bytes.Clone(value))
		}
	}
	server.values = append(server.values, pending...)
	server.index = call.Args().Index()
	return nil
}

/* Done reads without consuming; replay position and advancement belong to the graph. */
func (server *SequenceServer) Done(ctx context.Context, call Sequence_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "sequence: allocate result", err))
	}
	results.SetCount(uint64(len(server.values)))
	results.SetMissing()

	if server.index >= uint64(len(server.values)) {
		return nil
	}
	results.SetFound(true)
	results.SetItem()
	results.Item().SetNext(server.index + 1)

	if err := results.Item().SetOut(server.values[server.index]); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "sequence: set value", err))
	}
	return nil
}
