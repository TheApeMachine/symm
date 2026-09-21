package store

import (
	"bytes"
	"context"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
)

/*
RadixServer retains one value per key and reads it back. A metric composes it
to keep its own local state for each symbol it observes, so the state belongs
to the graph rather than hiding inside an operation.

A key nothing has been retained under is reported as not found rather than as
an empty value, so a symbol never observed stays distinguishable from one
observed as empty.
*/
type RadixServer struct {
	root  atomic.Pointer[iradix.Tree[[]byte]]
	out   []byte
	found bool
}

func NewRadix() *RadixServer {
	server := &RadixServer{}
	server.root.Store(iradix.New[[]byte]())

	return server
}

/*
Write retains a value under a key, or reads that key back when querying.
*/
func (server *RadixServer) Write(ctx context.Context, call Radix_write) error {
	key, err := call.Args().Key()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[store.radix.Write] failed to read key argument",
			err,
		))
	}

	if key == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"[store.radix.Write] key is not defined",
			nil,
		))
	}

	if call.Args().Query() {
		server.out, server.found = server.root.Load().Get([]byte(key))
		return nil
	}

	value, err := call.Args().Value()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.BadRequest,
			"[store.radix.Write] failed to read value argument",
			err,
		))
	}

	retained := bytes.Clone(value)
	updated, _, _ := server.root.Load().Insert([]byte(key), retained)
	server.root.Store(updated)

	server.out = retained
	server.found = true

	return nil
}

/*
Done emits the value resolved for the key and whether it was present.
*/
func (server *RadixServer) Done(ctx context.Context, call Radix_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[store.radix.Done] failed to allocate results",
			err,
		))
	}

	results.SetFound(server.found)

	if server.found && len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[store.radix.Done] failed to set out",
				err,
			))
		}
	}

	server.out = nil
	server.found = false

	return nil
}
