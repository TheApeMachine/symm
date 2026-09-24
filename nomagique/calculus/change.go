package calculus

import (
	"context"
	"fmt"

	"github.com/theapemachine/errnie"
)

/*
ChangeServer is how far each element of a list moved since its previous reading.
*/
type ChangeServer struct {
	arrived bool
	change  []float64
	defined []bool
	index   []int64
	latest  []float64
}

func NewChange() *ChangeServer {
	return &ChangeServer{}
}

func (server *ChangeServer) Write(ctx context.Context, call Change_write) error {
	args := call.Args()
	value, err := args.Value()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "calculus.change: failed to read value", err))
	}

	present, err := args.Present()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "calculus.change: failed to read present", err))
	}

	previous, err := args.Previous()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "calculus.change: failed to read previous", err))
	}

	known, err := args.Known()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "calculus.change: failed to read known", err))
	}

	if present.Len() != value.Len() || known.Len() != previous.Len() {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			fmt.Sprintf(
				"calculus.change: %d values with %d presence flags, %d previous with %d known flags",
				value.Len(), present.Len(), previous.Len(), known.Len(),
			),
			nil,
		))
	}

	server.arrived = value.Len() > 0
	server.change = make([]float64, value.Len())
	server.defined = make([]bool, value.Len())
	server.index = server.index[:0]
	server.latest = server.latest[:0]

	for element := range value.Len() {
		if !present.At(element) {
			continue
		}

		reading := value.At(element)
		server.index = append(server.index, int64(element))
		server.latest = append(server.latest, reading)

		if element >= known.Len() || !known.At(element) {
			continue
		}

		server.change[element] = reading - previous.At(element)
		server.defined[element] = true
	}

	return nil
}

func (server *ChangeServer) Done(ctx context.Context, call Change_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "calculus.change: failed to allocate results", err))
	}

	change, err := results.NewChange(int32(len(server.change)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "calculus.change: failed to allocate change", err))
	}

	defined, err := results.NewDefined(int32(len(server.defined)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "calculus.change: failed to allocate defined", err))
	}

	for element, moved := range server.change {
		change.Set(element, moved)
		defined.Set(element, server.defined[element])
	}

	results.SetIdle()

	if !server.arrived {
		server.change, server.defined = nil, nil
		return nil
	}

	results.SetRead()
	read := results.Read()
	index, err := read.NewIndex(int32(len(server.index)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "calculus.change: failed to allocate index", err))
	}

	latest, err := read.NewLatest(int32(len(server.latest)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "calculus.change: failed to allocate latest", err))
	}

	for position, element := range server.index {
		index.Set(position, element)
		latest.Set(position, server.latest[position])
	}

	server.change, server.defined, server.arrived = nil, nil, false
	server.index, server.latest = server.index[:0], server.latest[:0]
	return nil
}
