package compiler

import (
	"bytes"
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/runtime"
)

/* Snapshot reads all snapshot-capable state owners behind one evaluation fence. */
func (p *Program) Snapshot(ctx context.Context, call runtime.Snapshot_snapshot) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	var owners []CompiledNode
	for _, node := range p.Nodes {
		if Implements(node.Identity.InterfaceID, runtime.Snapshot_TypeID) {
			owners = append(owners, node)
		}
	}
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return errnie.Error(err)
	}
	defer message.Release()
	snapshot, err := runtime.NewRootSnapshotSet(segment)
	if err != nil {
		return errnie.Error(err)
	}
	if err := snapshot.SetVersion(p.Version); err != nil {
		return errnie.Error(err)
	}
	entries, err := snapshot.NewEntries(int32(len(owners)))
	if err != nil {
		return errnie.Error(err)
	}
	for index, node := range owners {
		future, release := runtime.Snapshot(node.Client).Snapshot(ctx, nil)
		result, err := future.Struct()
		if err != nil {
			release()
			return errnie.Error(err)
		}
		encoded, err := result.Data()
		if err == nil {
			entry := entries.At(index)
			entry.SetInterfaceId(node.Identity.InterfaceID)
			err = entry.SetName(node.ID)
			if err == nil {
				err = entry.SetConfiguration(node.Identity.ConfigDigest[:])
			}
			if err == nil {
				err = entry.SetData(encoded)
			}
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

/* Restore rejects a changed state-owner universe before restoring any node. */
func (p *Program) Restore(ctx context.Context, call runtime.Snapshot_restore) error {
	p.mu.Lock()
	defer p.mu.Unlock()
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
	if version != p.Version {
		return errnie.Error(errnie.Err(errnie.Validation, "program: checkpoint belongs to another graph", nil))
	}
	entries, err := snapshot.Entries()
	if err != nil {
		return errnie.Error(err)
	}
	owners := make(map[string]CompiledNode)
	for _, node := range p.Nodes {
		if Implements(node.Identity.InterfaceID, runtime.Snapshot_TypeID) {
			owners[node.ID] = node
		}
	}
	if len(owners) != entries.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, "program: checkpoint state owners changed", nil))
	}
	for index := range entries.Len() {
		entry := entries.At(index)
		name, err := entry.Name()
		if err != nil {
			return errnie.Error(err)
		}
		node, found := owners[name]
		configuration, err := entry.Configuration()
		if err != nil {
			return errnie.Error(err)
		}
		if !found || node.Identity.InterfaceID != entry.InterfaceId() || !bytes.Equal(configuration, node.Identity.ConfigDigest[:]) {
			return errnie.Error(errnie.Err(errnie.Validation, "program: checkpoint owner configuration differs: "+name, nil))
		}
		delete(owners, name)
	}
	for index := range entries.Len() {
		entry := entries.At(index)
		name, err := entry.Name()
		if err != nil {
			return errnie.Error(err)
		}
		encoded, err := entry.Data()
		if err != nil {
			return errnie.Error(err)
		}
		node := p.Nodes[p.NodeMap[name]]
		future, release := runtime.Snapshot(node.Client).Restore(ctx, func(params runtime.Snapshot_restore_Params) error { return params.SetData(encoded) })
		_, err = future.Struct()
		release()
		if err != nil {
			return errnie.Error(err)
		}
	}
	return nil
}
