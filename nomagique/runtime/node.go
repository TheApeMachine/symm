package runtime

import (
	"context"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

type Node[T any] interface {
	Step(T) T
	Register() T
}

/*
Consumer binds one Node to the Workload's register. At construction the node
identifies itself: Register() supplies the initial value if supported, an identify query
appends it and answers the slot, and the node is told its identity so the
values it produces name their own register slot.
*/
type Consumer[T any] struct {
	*System
	node      Node[T]
	register  *store.Register[T]
	tees      []Tee
	ID        int
	peerLimit int
}

func NewConsumer[T any](
	ctx context.Context,
	node Node[T],
	register *store.Register[T],
	tees ...Tee,
) *Consumer[T] {
	consumer := &Consumer[T]{
		node:     node,
		register: register,
		tees:     tees,
	}

	consumer.System = NewSystem(ctx, "consumer", consumer)

	if err := errnie.Error(errnie.Require(map[string]any{
		"system":   consumer.System,
		"node":     consumer.node,
		"register": consumer.register,
	})); err != nil {
		return nil
	}

	// This will obtain a slot in the register, and call Identify on both the
	// consumer as well as the measurement (obtained via node.Register()), so both
	// will be stamped with the same ID, corresponding to the register slot index.
	data.Read[*store.Query[T]](consumer.register.Next(data.NewValue(*store.NewQuery(
		consumer, data.ActionIdentify, data.NewValue(node.Register()),
	))))

	return consumer
}

/*
Identity names the register slot this consumer's node owns, so the consumer
is the subject of every query against its slot.
*/
func (consumer *Consumer[T]) Identity() int {
	return consumer.ID
}

/*
Identify names the register slot this consumer's node owns, so the consumer
is the subject of every query against its slot.
*/
func (consumer *Consumer[T]) Identify(id int) data.Identifiable[T] {
	consumer.ID = id
	return consumer
}

/*
Handle steps one node over every slot in [lower, upper]. Each invocation
reads the node's registered data back out of the register, passes it to Step,
and puts what Step returns back into the register under the node's slot.
Before READY, Handle drops the range without stepping or publishing.
*/
func (consumer *Consumer[T]) Handle(lower, upper int64) {
	if consumer.Status() != READY {
		errnie.Warn(consumer.Name() + ": Handle called before READY; dropping event")
		return
	}

	for seq := lower; seq <= upper; seq++ {
		val := consumer.register.Next(data.NewValue(
			*store.NewQuery(consumer, data.ActionRead),
		))

		if data.Read[*data.Measurement[float64]](val) == nil {
			consumer.Error(errnie.Err(
				errnie.NotFound,
				"[consumer] no measurement found for consumer",
				nil,
			))
		}

		result := consumer.node.Step(data.Read[T](val))

		out := data.Read[*data.Measurement[float64]](consumer.register.Next(
			data.NewValue(*store.NewQuery(
				consumer, data.ActionWrite, data.NewValue(result),
			)),
		))

		for _, tee := range consumer.tees {
			tee.Push(out)
		}
	}
}
