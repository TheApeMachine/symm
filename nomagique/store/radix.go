package store

import (
	"bytes"
	"context"
	"fmt"
	"sync/atomic"

	capnp "capnproto.org/go/capnp/v3"
	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
)

/* RadixServer owns one immutable key/value tree and atomic batch replacement. */
type RadixServer struct {
	root  atomic.Pointer[iradix.Tree[[]byte]]
	out   [][]byte
	found []bool
}

func NewRadix() *RadixServer {
	server := &RadixServer{}
	server.root.Store(iradix.New[[]byte]())
	return server
}

/* Write selects prior values and atomically applies a complete supplied batch. */
func (server *RadixServer) Write(ctx context.Context, call Radix_write) error {
	keys, err := call.Args().Key()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "radix: keys", err))
	}

	values, err := call.Args().Value()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "radix: values", err))
	}

	if call.Args().HasValue() && (keys.Len() == 0 || values.Len() != keys.Len()) {
		return errnie.Error(errnie.Err(errnie.Validation, "radix: replacement batch must match all requested keys", nil))
	}

	tree := server.root.Load()
	var transaction *iradix.Txn[[]byte]

	if call.Args().HasValue() {
		transaction = tree.Txn()
	}
	seen := make(map[string]bool, keys.Len())
	server.out = make([][]byte, keys.Len())
	server.found = make([]bool, keys.Len())

	for index := range keys.Len() {
		key, err := keys.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "radix: key", err))
		}

		if key == "" || seen[key] {
			fmt.Printf("RADIX WRITE FAILED: index=%d, key=%q, seen=%v, keysLen=%d\n", index, key, seen[key], keys.Len())
			return errnie.Error(errnie.Err(errnie.Validation, "radix: keys must be nonempty and distinct", nil))
		}

		seen[key] = true
		server.out[index], server.found[index] = tree.Get([]byte(key))

		if !call.Args().HasValue() {
			continue
		}

		pointer, err := capnp.PointerList(values).At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "radix: replacement pointer", err))
		}

		if len(pointer.Data()) == 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "radix: every replacement slot must be nonempty", nil))
		}

		transaction.Insert([]byte(key), bytes.Clone(pointer.Data()))
	}

	if call.Args().HasValue() {
		server.root.Store(transaction.Commit())
	}

	return nil
}

/* Done emits the selected revision and clears only the transient result. */
func (server *RadixServer) Done(ctx context.Context, call Radix_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "radix: allocate result", err))
	}

	values, err := results.NewOut(int32(len(server.out)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "radix: allocate values", err))
	}

	found, err := results.NewFound(int32(len(server.found)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "radix: allocate presence", err))
	}

	for index, value := range server.out {
		found.Set(index, server.found[index])

		if err := values.Set(index, value); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "radix: emit value", err))
		}
	}

	server.out = nil
	server.found = nil
	return nil
}
