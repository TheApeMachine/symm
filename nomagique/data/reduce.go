package data

import (
	"context"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/*
ReduceServer folds the values it is given into one, and publishes the fold
when it is told the collection has ended. Composed downstream of Iterate it
turns a collection into a single value, which is the other half of walking a
collection without any node taking a function.

The fold carries no value until it has seen one, so an empty collection
reports ready=false rather than a zero, and a sum is never confused with
nothing having arrived.
*/
type ReduceServer struct {
	*runtime.System
	operator    string
	accumulator float64
	count       int64
	out         float64
	published   int64
	ready       bool
}

func NewReduce(ctx context.Context) *ReduceServer {
	server := &ReduceServer{
		System:   runtime.NewSystem(ctx, "data.reduce"),
		operator: "sum",
	}

	server.Transition(runtime.READY)
	return server
}

/*
Write folds one value into the accumulator, publishing it on flush.
*/
func (server *ReduceServer) Write(ctx context.Context, call Reduce_write) error {
	operator, err := call.Args().Operator()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[data.reduce.Write] failed to read operator argument",
			err,
		))
	}

	if len(operator) > 0 {
		server.operator = operator
	}

	if err := server.fold(call.Args().Value()); err != nil {
		return err
	}

	server.ready = false

	if !call.Args().Flush() {
		return nil
	}

	return server.publish()
}

/*
Done emits the fold once the collection it covers has ended.
*/
func (server *ReduceServer) Done(ctx context.Context, call Reduce_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[data.reduce.Done] failed to allocate results",
			err,
		))
	}

	results.SetStatus(runtime.Status(server.Status()))
	results.SetReady(server.ready)
	results.SetCount(server.published)

	if server.ready {
		results.SetOut(server.out)
	}

	server.ready = false
	return nil
}

/*
fold accumulates one value under the configured operator.
*/
func (server *ReduceServer) fold(value float64) error {
	if server.count == 0 {
		switch strings.TrimSpace(server.operator) {
		case "sum", "mean", "min", "max", "product", "count":
			server.accumulator = value
			server.count = 1
			return nil
		}

		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.reduce] operator "+server.operator+" is not defined",
			nil,
		))
	}

	switch strings.TrimSpace(server.operator) {
	case "sum", "mean":
		server.accumulator += value
	case "product":
		server.accumulator *= value
	case "min":
		server.accumulator = math.Min(server.accumulator, value)
	case "max":
		server.accumulator = math.Max(server.accumulator, value)
	case "count":
	default:
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[data.reduce] operator "+server.operator+" is not defined",
			nil,
		))
	}

	server.count++
	return nil
}

/*
publish emits the fold and starts the next one.
*/
func (server *ReduceServer) publish() error {
	if server.count == 0 {
		return nil
	}

	switch strings.TrimSpace(server.operator) {
	case "mean":
		server.out = server.accumulator / float64(server.count)
	case "count":
		server.out = float64(server.count)
	default:
		server.out = server.accumulator
	}

	server.published = server.count
	server.ready = true
	server.accumulator = 0
	server.count = 0

	return nil
}
