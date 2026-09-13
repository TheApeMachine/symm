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
	node     Node[T]
	register *store.Register[T]
	ID       int
}

func NewConsumer[T any](
	node Node[T], register *store.Register[T],
) *Consumer[T] {
	consumer := &Consumer[T]{
		node:     node,
		register: register,
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
		if sys, ok := any(consumer.node).(interface{ Status() Stage }); ok && sys.Status() != READY {
			continue
		}

		query := store.NewQuery(consumer, data.ActionRead)
		val := data.Read[T](consumer.register.Next(data.NewValue(*query)))

		valToStep := val

		if measurement, ok := any(val).(*data.Measurement[float64]); ok && measurement != nil {
			valToStep = any(measurement.Clone()).(T)
		}

		result := consumer.node.Step(valToStep)
		toWrite := result

		if measurement, ok := any(result).(*data.Measurement[float64]); ok && measurement != nil {
			toWrite = any(measurement.Clone()).(T)
		}

		data.Read[T](consumer.register.Next(data.NewValue(*store.NewQuery(
			consumer, data.ActionWrite, toWrite,
		))))
	}
}
