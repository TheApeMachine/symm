package runtime

import (
	"github.com/theapemachine/symm/nomagique/data"
	sequence "github.com/theapemachine/symm/nomagique/data/sequence"
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
	ID        int
	peerLimit int
	tees      []Tee
	owner     string
}

func NewConsumer[T any](
	node Node[T], register *store.Register[T], tees ...Tee,
) *Consumer[T] {
	consumer := &Consumer[T]{
		node:      node,
		register:  register,
		peerLimit: -1,
		tees:      tees,
	}

	if node == nil || register == nil {
		return consumer
	}

	val := node.Register()

	if named, ok := any(node).(interface{ Name() string }); ok {
		consumer.owner = named.Name()
	}
	sequence.
		Read[*store.Query[T]](consumer.register.Next(sequence.NewValue(*store.NewQuery(
		consumer, data.ActionIdentify, sequence.NewValue(val),
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
Measurement sequences are one-based because zero means unstamped. Nil outputs
clear that sequence slot without replacing the node's last working state.
*/
func (consumer *Consumer[T]) Handle(lower, upper int64) {
	for sequence := lower; sequence <= upper; sequence++ {
		result := consumer.step(sequence)
		payload := sequence.NewValue(result)

		if measurement, ok := any(result).(*data.Measurement[float64]); any(result) == nil || (ok && measurement == nil) {
			payload = nil
		}

		query := store.NewQuery(consumer, data.ActionWrite, payload).SetSequence(sequence)

		out := sequence.Read[T](consumer.register.Next(sequence.NewValue(*query)))
		measurement, ok := any(out).(*data.Measurement[float64])

		if !ok || measurement == nil {
			continue
		}

		for _, tee := range consumer.tees {
			if tee != nil {
				tee.Push(measurement)
			}
		}
	}
}

func (consumer *Consumer[T]) step(sequence int64) T {
	var empty T
	query := store.NewQuery(consumer, data.ActionRead).SetSequence(sequence)

	if consumer.peerLimit >= 0 {
		query.SetPeerLimit(consumer.peerLimit)
	}

	value := sequence.Read[T](consumer.register.Next(sequence.NewValue(*query)))

	if measurement, ok := any(value).(*data.Measurement[float64]); ok && measurement != nil {
		peers := measurement.Peers[:0]

		for _, peer := range measurement.Peers {
			if peer != nil && peer.SeqIdx == sequence+1 {
				peers = append(peers, peer)
			}
		}

		measurement.Peers = peers

		if consumer.peerLimit != 0 && measurement.Metadata["peer-interest"] != "" && len(peers) == 0 {
			return empty
		}

		measurement.SeqIdx = sequence + 1
		measurement.Result = nil
		measurement.Err = nil
	}

	result := consumer.node.Step(value)

	if measurement, ok := any(result).(*data.Measurement[float64]); ok && measurement != nil {
		measurement.SeqIdx = sequence + 1
		measurement.Provenance["owner"] = consumer.owner
	}

	return result
}
