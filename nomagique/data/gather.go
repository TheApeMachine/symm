package data

import (
	"context"
	"fmt"

	"github.com/theapemachine/errnie"
)

/*
GatherServer holds, as one list, the numbers its producers delivered on this
evaluation.
*/
type GatherServer struct {
	values  []float64
	present []bool
}

func NewGather() *GatherServer {
	return &GatherServer{}
}

func (server *GatherServer) Write(ctx context.Context, call Gather_write) error {
	values, err := call.Args().Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: failed to read values", err))
	}

	present, err := call.Args().Present()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "data.gather: failed to read present", err))
	}

	server.values, server.present = server.values[:0], server.present[:0]

	if values.Len() == 0 {
		return nil
	}

	if present.Len() != values.Len() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf("data.gather: %d values with %d presence flags", values.Len(), present.Len()),
			nil,
		))
	}

	for slot := range values.Len() {
		server.values = append(server.values, values.At(slot))
		server.present = append(server.present, present.At(slot))
	}

	return nil
}

func (server *GatherServer) Done(ctx context.Context, call Gather_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: failed to allocate results", err))
	}

	values, err := results.NewValues(int32(len(server.values)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: failed to allocate values", err))
	}

	present, err := results.NewPresent(int32(len(server.present)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "data.gather: failed to allocate present", err))
	}

	for slot, value := range server.values {
		values.Set(slot, value)
		present.Set(slot, server.present[slot])
	}

	server.values, server.present = server.values[:0], server.present[:0]
	return nil
}
