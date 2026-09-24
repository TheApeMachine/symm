package store

import (
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/theapemachine/errnie"
)

/*
TrailServer keeps the most recent arrivals as a JSON array.
*/
type TrailServer struct {
	kept    [][]byte
	arrived bool
	scope   string
}

func NewTrail() *TrailServer {
	return &TrailServer{}
}

func (server *TrailServer) Write(ctx context.Context, call Trail_write) error {
	args := call.Args()
	capacity := int(args.Capacity())

	if capacity == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "store.trail: capacity must be declared", nil))
	}

	scopes, err := args.Scope()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.trail: failed to read scope", err))
	}

	if scopes.Len() > 0 {
		scope, err := scopes.At(0)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "store.trail: failed to read a scope", err))
		}

		// A new series shows none of the previous one.
		if scope != server.scope {
			server.scope = scope
			server.kept = nil
		}
	}

	values, err := args.Values()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.trail: failed to read values", err))
	}

	numbers, err := args.Numbers()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.trail: failed to read numbers", err))
	}

	present, err := args.Present()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.trail: failed to read present", err))
	}

	for index := range values.Len() {
		value, err := values.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "store.trail: failed to read a value", err))
		}

		if len(value) == 0 {
			continue
		}

		if !json.Valid(value) {
			return errnie.Error(errnie.Err(errnie.Validation, "store.trail: a value is not JSON", nil))
		}

		server.keep(bytes.Clone(value), capacity)
	}

	for index := range numbers.Len() {
		if index >= present.Len() || !present.At(index) {
			continue
		}

		server.keep([]byte(strconv.FormatFloat(numbers.At(index), 'g', -1, 64)), capacity)
	}

	return nil
}

func (server *TrailServer) keep(value []byte, capacity int) {
	server.kept = append(server.kept, value)
	server.arrived = true

	if len(server.kept) > capacity {
		server.kept = server.kept[len(server.kept)-capacity:]
	}
}

func (server *TrailServer) Done(ctx context.Context, call Trail_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "store.trail: failed to allocate results", err))
	}

	results.SetIdle()

	if !server.arrived {
		return nil
	}

	encoded := append([]byte{'['}, bytes.Join(server.kept, []byte{','})...)
	encoded = append(encoded, ']')

	if err := results.SetOut(encoded); err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "store.trail: failed to set out", err))
	}

	server.arrived = false
	return nil
}
