package runtime

import (
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/store"
)

type Node[T any] interface {
	Step(T) T
}

/*
Consumer binds one Node to the Workload's register. At construction the node
identifies itself: Register() supplies the initial value if supported, an identify query
appends it and answers the slot, and the node is told its identity so the
values it produces name their own register slot.
*/
type Consumer[T any] struct {
	node          Node[T]
	register      *store.Register[T]
	ID            int
	peerInterests []string
}

func NewConsumer[T any](
	node Node[T], register *store.Register[T],
) *Consumer[T] {
	consumer := &Consumer[T]{
		node:     node,
		register: register,
	}

	if register == nil {
		return consumer
	}

	if regNode, ok := node.(interface{ Register() (T, []string) }); ok {
		val, peers := regNode.Register()
		consumer.peerInterests = peers

		data.Read[*store.Query[T]](consumer.register.Next(data.NewValue(*store.NewQuery(
			consumer, data.ActionIdentify, val,
		))))

		if m, ok := any(val).(*data.Measurement[float64]); ok && m != nil {
			m.ID = consumer.ID
		}

		return consumer
	}

	if regNode, ok := node.(interface{ Register() T }); ok {
		val := regNode.Register()

		data.Read[*store.Query[T]](consumer.register.Next(data.NewValue(*store.NewQuery(
			consumer, data.ActionIdentify, val,
		))))

		if m, ok := any(val).(*data.Measurement[float64]); ok && m != nil {
			m.ID = consumer.ID
		}
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
When peer interests are declared, Consumer queries those peers from the register
using store.Query and packages them into Peers before calling Step.
*/
func (consumer *Consumer[T]) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		if sys, ok := any(consumer.node).(interface{ Status() Stage }); ok && sys.Status() != READY {
			continue
		}

		query := store.NewQuery(consumer, data.ActionRead)
		val := data.Read[T](consumer.register.Next(data.NewValue(*query)))

		if len(consumer.peerInterests) > 0 {
			consumer.populatePeers(val)
		}

		result := consumer.node.Step(val)
		toWrite := result

		if measurement, ok := any(result).(*data.Measurement[float64]); ok && measurement != nil {
			toWrite = any(measurement.Clone()).(T)
		}

		data.Read[T](consumer.register.Next(data.NewValue(*store.NewQuery(
			consumer, data.ActionWrite, toWrite,
		))))
	}
}

func (consumer *Consumer[T]) populatePeers(val T) {
	measurement, ok := any(val).(*data.Measurement[float64])

	if !ok || measurement == nil {
		return
	}

	measurement.Peers = measurement.Peers[:0]
	query := store.NewQuery[T](nil, data.ActionRead)

	for peer := range data.ReadSeq[T](consumer.register.Next(data.NewValue(*query))) {
		peerMeasurement, ok := any(peer).(*data.Measurement[float64])

		if !ok || peerMeasurement == nil || peerMeasurement == measurement || peerMeasurement.ID == consumer.ID {
			continue
		}

		if consumer.wantsPeer(peerMeasurement) {
			measurement.Peers = append(measurement.Peers, peerMeasurement)
		}
	}
}

func (consumer *Consumer[T]) wantsPeer(peer *data.Measurement[float64]) bool {
	for _, interest := range consumer.peerInterests {
		if interest == "*" || interest == peer.Source || interest == peer.Label {
			return true
		}
	}

	return false
}
