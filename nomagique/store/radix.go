package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"

	"github.com/theapemachine/symm/nomagique/runtime"

	capnp "capnproto.org/go/capnp/v3"
	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
)

/* RadixServer owns one immutable key/value tree and atomic batch replacement. */
type RadixServer struct {
	root     atomic.Pointer[iradix.Tree[[]byte]]
	out      [][]byte
	found    []bool
	path     string
	contract string
	revision uint64
	saved    uint64
	extent   uint64
}

func NewRadix() *RadixServer {
	server := &RadixServer{}
	server.root.Store(iradix.New[[]byte]())
	return server
}

/* Write selects prior values and atomically applies a complete supplied batch. */
func (server *RadixServer) Write(ctx context.Context, call Radix_write) error {
	contract, err := call.Args().Contract()
	if err != nil {
		return errnie.Error(err)
	}
	if contract != "" && contract != server.contract {
		if server.path != "" || server.revision > 0 || server.contract != "" {
			return errnie.Error(errnie.Err(errnie.Validation, "radix: checkpoint contract cannot change after initialization", nil))
		}
		server.contract = contract
	}
	path, err := call.Args().Path()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "radix: checkpoint path", err))
	}
	if path != "" && path != server.path {
		if server.path != "" || server.revision > 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "radix: checkpoint cannot change after initialization", nil))
		}
		if err := server.load(path); err != nil {
			return err
		}
		server.path = path
	}
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
	extent := server.extent
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

		if key == "" {
			server.out[index] = nil
			server.found[index] = false
			continue
		}

		if seen[key] {
			return errnie.Error(errnie.Err(errnie.Validation, fmt.Sprintf("radix: keys must be distinct (index %d: duplicate key %q)", index, key), nil))
		}

		seen[key] = true
		server.out[index], server.found[index] = tree.Get([]byte(key))
		if call.Args().Prefix() && !call.Args().HasValue() {
			_, server.out[index], server.found[index] = tree.Root().LongestPrefix([]byte(key))
		}

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
		extent = max(extent, uint64(len(key)))
	}

	if call.Args().HasValue() {
		server.root.Store(transaction.Commit())
		server.extent = extent
		server.revision++
	}

	return nil
}

/* Done emits the selected revision and clears only the transient result. */
func (server *RadixServer) Done(ctx context.Context, call Radix_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "radix: allocate result", err))
	}

	results.SetDurable(server.path != "")

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

/* load restores an explicitly configured model; malformed checkpoints never become empty trees. */
func (server *RadixServer) load(path string) error {
	encoded, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "radix: read checkpoint", err))
	}
	message, err := capnp.Unmarshal(encoded)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "radix: decode checkpoint", err))
	}
	defer message.Release()
	snapshot, err := ReadRootRadixSnapshot(message)
	if err != nil {
		return errnie.Error(err)
	}
	format, err := snapshot.Format()
	if err != nil {
		return errnie.Error(err)
	}
	if format != "symm.radix/1" {
		return errnie.Error(errnie.Err(errnie.Validation, "radix: unknown checkpoint format", nil))
	}
	contract, err := snapshot.Contract()
	if err != nil {
		return errnie.Error(err)
	}
	if contract != server.contract {
		return errnie.Error(errnie.Err(errnie.Validation, "radix: stored key/value contract differs from the authored graph", nil))
	}
	keys, err := snapshot.Keys()
	if err != nil {
		return errnie.Error(err)
	}
	values, err := snapshot.Values()
	if err != nil {
		return errnie.Error(err)
	}
	if keys.Len() != values.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, "radix: checkpoint keys and values differ", nil))
	}
	transaction := iradix.New[[]byte]().Txn()
	previous := ""
	var extent uint64
	for index := range keys.Len() {
		key, err := keys.At(index)
		if err != nil {
			return errnie.Error(err)
		}
		value, err := values.At(index)
		if err != nil {
			return errnie.Error(err)
		}
		if key == "" || key <= previous || len(value) == 0 {
			return errnie.Error(errnie.Err(errnie.Validation, "radix: invalid checkpoint entry", nil))
		}
		transaction.Insert([]byte(key), bytes.Clone(value))
		extent = max(extent, uint64(len(key)))
		previous = key
	}
	server.root.Store(transaction.Commit())
	server.extent = extent
	server.revision = snapshot.Revision()
	server.saved = server.revision
	return nil
}

/* Flush atomically replaces the configured checkpoint after syncing its bytes. */
func (server *RadixServer) Flush(ctx context.Context, call runtime.Durable_flush) error {
	if server.path == "" || server.saved == server.revision {
		return nil
	}
	tree := server.root.Load()
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return errnie.Error(err)
	}
	defer message.Release()
	snapshot, err := NewRootRadixSnapshot(segment)
	if err != nil {
		return errnie.Error(err)
	}
	if err := snapshot.SetFormat("symm.radix/1"); err != nil {
		return errnie.Error(err)
	}
	if err := snapshot.SetContract(server.contract); err != nil {
		return errnie.Error(err)
	}
	snapshot.SetRevision(server.revision)
	keys, err := snapshot.NewKeys(int32(tree.Len()))
	if err != nil {
		return errnie.Error(err)
	}
	values, err := snapshot.NewValues(int32(tree.Len()))
	if err != nil {
		return errnie.Error(err)
	}
	index := 0
	tree.Root().Walk(func(key []byte, value []byte) bool {
		if err = keys.Set(index, string(key)); err != nil {
			return true
		}
		if err = values.Set(index, value); err != nil {
			return true
		}
		index++
		return false
	})
	if err != nil {
		return errnie.Error(err)
	}
	encoded, err := message.Marshal()
	if err != nil {
		return errnie.Error(err)
	}
	directory := filepath.Dir(server.path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "radix: checkpoint directory", err))
	}
	temporary, err := os.CreateTemp(directory, ".radix-*")
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "radix: checkpoint file", err))
	}
	_, err = temporary.Write(encoded)
	err = errors.Join(err, temporary.Sync(), temporary.Close())
	if err == nil {
		err = os.Rename(temporary.Name(), server.path)
	}
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "radix: commit checkpoint", errors.Join(err, os.Remove(temporary.Name()))))
	}
	parent, err := os.Open(directory)
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "radix: open checkpoint directory", err))
	}
	err = errors.Join(parent.Sync(), parent.Close())
	if err != nil {
		return errnie.Error(errnie.Err(errnie.IO, "radix: sync checkpoint directory", err))
	}
	server.saved = server.revision
	return nil
}

/* Load reads one complete checkpoint from this node's durable revision. */
func (server *RadixServer) Load(ctx context.Context, call runtime.Checkpoint_load) error {
	key, err := call.Args().Key()
	if err != nil {
		return errnie.Error(err)
	}
	if key == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "radix: checkpoint key is required", nil))
	}
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	value, _ := server.root.Load().Get([]byte(key))
	return errnie.Error(result.SetData(value))
}

/* Save atomically replaces and commits a complete checkpoint through the existing tree. */
func (server *RadixServer) Save(ctx context.Context, call runtime.Checkpoint_save) error {
	key, err := call.Args().Key()
	if err != nil {
		return errnie.Error(err)
	}
	value, err := call.Args().Data()
	if err != nil {
		return errnie.Error(err)
	}
	if key == "" || len(value) == 0 || server.path == "" {
		return errnie.Error(errnie.Err(errnie.Validation, "radix: durable checkpoint needs a key, state and configured path", nil))
	}
	tree, _, _ := server.root.Load().Insert([]byte(key), bytes.Clone(value))
	server.root.Store(tree)
	server.revision++
	return server.Flush(ctx, runtime.Durable_flush{})
}

/* Measure reports the representation bound of actual learned keys. */
func (server *RadixServer) Measure(ctx context.Context, call Radix_measure) error {
	result, err := call.AllocResults()
	if err != nil {
		return errnie.Error(err)
	}
	result.SetExtent(server.extent)
	return nil
}
