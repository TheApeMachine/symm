package compiler

import (
	"context"
	"crypto/sha256"
	"fmt"
	"maps"
	"slices"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* Snapshot preserves every owned market graph in deterministic partition order. */
func (factory *stageFactory) Snapshot(ctx context.Context, call runtime.Snapshot_snapshot) error {
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return errnie.Error(err)
	}
	defer message.Release()
	snapshot, err := runtime.NewRootSnapshotSet(segment)

	if err != nil {
		return errnie.Error(err)
	}
	if err = snapshot.SetVersion(fmt.Sprintf("%x/%s/%s/%s", sha256.Sum256(factory.graph), factory.producer, factory.node, factory.field)); err != nil {
		return errnie.Error(err)
	}
	names := slices.Sorted(maps.Keys(factory.children))
	entries, err := snapshot.NewEntries(int32(len(names)))

	if err != nil {
		return errnie.Error(err)
	}
	for index, name := range names {
		entry := entries.At(index)
		if err = entry.SetName(name); err != nil {
			return errnie.Error(err)
		}
		entry.SetInterfaceId(runtime.State_TypeID)
		future, release := runtime.Snapshot(factory.children[name]).Snapshot(ctx, nil)
		result, err := future.Struct()
		if err != nil {
			release()
			return errnie.Error(err)
		}
		encoded, err := result.Data()
		if err == nil {
			err = entry.SetData(encoded)
		}
		release()
		if err != nil {
			return errnie.Error(err)
		}
	}
	encoded, err := message.Marshal()

	if err != nil {
		return errnie.Error(err)
	}
	result, err := call.AllocResults()

	if err != nil {
		return errnie.Error(err)
	}
	return errnie.Error(result.SetData(encoded))
}

/* Restore constructs and validates all candidate graphs before replacing live ownership. */
func (factory *stageFactory) Restore(ctx context.Context, call runtime.Snapshot_restore) error {
	encoded, err := call.Args().Data()

	if err != nil {
		return errnie.Error(err)
	}
	message, err := capnp.Unmarshal(encoded)

	if err != nil {
		return errnie.Error(err)
	}
	defer message.Release()
	snapshot, err := runtime.ReadRootSnapshotSet(message)

	if err != nil {
		return errnie.Error(err)
	}
	version, err := snapshot.Version()

	if err != nil {
		return errnie.Error(err)
	}
	if version != fmt.Sprintf("%x/%s/%s/%s", sha256.Sum256(factory.graph), factory.producer, factory.node, factory.field) {
		return errnie.Error(errnie.Err(errnie.Validation, "factory: checkpoint belongs to another graph", nil))
	}
	entries, err := snapshot.Entries()

	if err != nil {
		return errnie.Error(err)
	}
	candidates := make(map[string]runtime.State, entries.Len())
	defer func() {
		for _, child := range candidates {
			child.Release()
		}
	}()
	for index := range entries.Len() {
		entry := entries.At(index)
		name, err := entry.Name()
		if err != nil {
			return errnie.Error(err)
		}
		if _, exists := candidates[name]; exists || name == "" || entry.InterfaceId() != runtime.State_TypeID {
			return errnie.Error(errnie.Err(errnie.Validation, "factory: invalid or duplicate checkpoint partition", nil))
		}
		encoded, err := entry.Data()
		if err != nil {
			return errnie.Error(err)
		}
		child, err := factory.create()
		if err != nil {
			return err
		}
		candidates[name] = child
		future, release := runtime.Snapshot(child).Restore(ctx, func(params runtime.Snapshot_restore_Params) error { return params.SetData(encoded) })
		_, err = future.Struct()
		release()
		if err != nil {
			return errnie.Error(err)
		}
	}
	previous := factory.children
	factory.children = candidates
	candidates = previous
	return nil
}
