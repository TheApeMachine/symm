package store

import (
	"bytes"
	"context"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

type RadixServer struct {
	root  atomic.Pointer[iradix.Tree[[]byte]]
	out   []byte
	found bool
}

func NewRadixServer() *RadixServer {
	server := &RadixServer{}
	server.root.Store(iradix.New[[]byte]())
	return server
}

func NewRadix() *RadixServer {
	return NewRadixServer()
}

func (server *RadixServer) Write(ctx context.Context, call Radix_write) error {
	key, _ := call.Args().Key()
	val, _ := call.Args().Value()

	current := server.root.Load()
	updated, _, _ := current.Insert([]byte(key), bytes.Clone(val))
	server.root.Store(updated)

	server.out = bytes.Clone(val)
	server.found = true
	return nil
}

func (server *RadixServer) Done(ctx context.Context, call Radix_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"radix: alloc results failed",
			err,
		))
	}

	if len(server.out) > 0 {
		if err := results.SetOut(server.out); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"radix: set out failed",
				err,
			))
		}
	}
	results.SetFound(server.found)

	server.out = nil
	server.found = false
	return nil
}
