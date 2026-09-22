package data

import (
	"context"

	"github.com/theapemachine/errnie"
)

/*
ElementServer takes one value out of a list by position.

Whether the position existed is reported alongside the value, because a list
shorter than expected and a list holding zero at that position are different
facts about whatever produced it, and answering zero for both would hide the
first behind the second.
*/
type ElementServer struct {
	out   float64
	found bool
}

func NewElement() *ElementServer {
	return &ElementServer{}
}

func (server *ElementServer) Write(ctx context.Context, call Element_write) error {
	args := call.Args()
	values, err := args.Values()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"data element: failed to read values",
			err,
		))
	}

	index := int(args.Index())
	server.out = 0
	server.found = false

	if index < 0 || index >= values.Len() {
		return nil
	}

	server.out = values.At(index)
	server.found = true
	return nil
}

func (server *ElementServer) Done(ctx context.Context, call Element_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"data element: failed to allocate results",
			err,
		))
	}

	results.SetOut(server.out)
	results.SetFound(server.found)
	return nil
}
