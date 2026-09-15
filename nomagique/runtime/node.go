package runtime

import (
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
	node      Node[T]
	register  *store.Register[T]
	tees      []any
	ID        int
	peerLimit int
}

func NewConsumer[T any](
	node Node[T], register *store.Register[T], tees ...any,
) *Consumer[T] {
	consumer := &Consumer[T]{
		node:      node,
		register:  register,
		tees:      tees,
		peerLimit: -1,
	}

	if node == nil || register == nil {
		return consumer
	}

	val := node.Register()

	data.Read[*store.Query[T]](consumer.register.Next(data.NewValue(*store.NewQuery(
		consumer, data.ActionIdentify, val,
	))))

	if meas, ok := any(val).(*data.Measurement[float64]); ok && meas != nil {
		meas.ID = consumer.ID
	}

	return consumer
}

/*
SetPeerLimit restricts the consumer's peer queries to register slots strictly below limit.
*/
func (consumer *Consumer[T]) SetPeerLimit(limit int) *Consumer[T] {
	consumer.peerLimit = limit
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
*/
func (consumer *Consumer[T]) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		query := store.NewQuery(consumer, data.ActionRead)

		if consumer.peerLimit >= 0 {
			query.SetPeerLimit(consumer.peerLimit)
		}

		val := data.Read[T](consumer.register.Next(data.NewValue(*query)))
		result := consumer.node.Step(val)

		out := data.Read[*data.Measurement[float64]](consumer.register.Next(data.NewValue(*store.NewQuery(
			consumer, data.ActionWrite, result,
		))))

		for _, tee := range consumer.tees {
			if pusher, ok := tee.(interface{ Push(*data.Measurement[float64]) }); ok {
				pusher.Push(out)
			}
		}
	}
}
