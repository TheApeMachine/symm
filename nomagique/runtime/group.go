package runtime

import (
	"context"

	"github.com/theapemachine/errnie"
)

/* GroupServer owns the membership of one native LMAX handler group. */
type GroupServer struct {
	consumers []Consumer
}

/* NewGroup constructs an unconfigured Cap'n Proto group. */
func NewGroup(ctx context.Context) *GroupServer { return &GroupServer{} }

/* Write fixes membership before a workspace acquires the group. */
func (server *GroupServer) Write(ctx context.Context, call Group_write) error {
	members, err := call.Args().Consumers()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "group: consumers", err))
	}

	if members.Len() == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "group: at least one consumer is required", nil))
	}

	if len(server.consumers) > 0 && len(server.consumers) != members.Len() {
		return errnie.Error(errnie.Err(errnie.Validation, "group: membership cannot change", nil))
	}

	for index := range members.Len() {
		consumer, err := members.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "group: consumer capability", err))
		}

		if !consumer.IsValid() {
			return errnie.Error(errnie.Err(errnie.Validation, "group: missing consumer capability", nil))
		}

		if len(server.consumers) > 0 && !server.consumers[index].IsSame(consumer) {
			return errnie.Error(errnie.Err(errnie.Validation, "group: membership cannot change", nil))
		}
	}

	if len(server.consumers) > 0 {
		return nil
	}

	for index := range members.Len() {
		consumer, err := members.At(index)

		if err != nil {
			return errnie.Error(errnie.Err(errnie.Validation, "group: consumer capability", err))
		}
		server.consumers = append(server.consumers, consumer.AddRef())
	}
	return nil
}

/* Done exposes configured membership to the graph. */
func (server *GroupServer) Done(ctx context.Context, call Group_done) error {
	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "group: allocate count", err))
	}
	results.SetCount(uint32(len(server.consumers)))
	return nil
}

/* Members supplies capabilities for independent native LMAX handlers. */
func (server *GroupServer) Members(ctx context.Context, call Group_members) error {
	if len(server.consumers) == 0 {
		return errnie.Error(errnie.Err(errnie.Validation, "group: not configured", nil))
	}

	results, err := call.AllocResults()

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "group: allocate members", err))
	}
	members, err := results.NewConsumers(int32(len(server.consumers)))

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "group: allocate consumer list", err))
	}

	for index, consumer := range server.consumers {
		if err := members.Set(index, consumer.AddRef()); err != nil {
			return errnie.Error(errnie.Err(errnie.Internal, "group: set consumer", err))
		}
	}
	return nil
}

/* Shutdown releases the capabilities retained by the group. */
func (server *GroupServer) Shutdown() {
	for _, consumer := range server.consumers {
		consumer.Release()
	}
}
