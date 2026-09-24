package store

import (
	"context"

	"github.com/theapemachine/errnie"
)

/*
HoldServer keeps the latest reading of a series.
*/
type HoldServer struct {
	value float64
	held  bool
	fresh bool
	scope string
}

func NewHold() *HoldServer {
	return &HoldServer{}
}

func (server *HoldServer) Write(ctx context.Context, call Hold_write) error {
	scope, err := call.Args().Scope()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.hold: failed to read scope", err))
	}

	if scope != server.scope {
		*server = HoldServer{scope: scope}
	}

	values, err := call.Args().Value()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.hold: failed to read value", err))
	}

	present, err := call.Args().Present()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "store.hold: failed to read present", err))
	}

	for slot := range values.Len() {
		if slot < present.Len() && !present.At(slot) {
			continue
		}

		server.value, server.held, server.fresh = values.At(slot), true, true
	}

	return nil
}

func (server *HoldServer) Done(ctx context.Context, call Hold_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "store.hold: failed to allocate results", err))
	}

	defer func() {
		server.fresh = false
	}()

	if !server.held {
		results.SetIdle()
		return nil
	}

	results.SetHeld()
	results.Held().SetValue(server.value)
	results.Held().SetFresh(server.fresh)
	return nil
}
