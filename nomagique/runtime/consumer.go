package runtime

import (
	"context"
	"iter"
	"sync/atomic"
	"unsafe"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
Slot is the pre-allocated event living in one ring position. The ring owns the
slot; a handler reads the payload out of it and must not retain the pointer
past its own call, because the producer reclaims the position once every
handler group has passed it.
*/
type Slot struct {
	Payload []byte
}

/*
Step is what a ring handler does with one observation: hand it to something
that processes it, and report what came back. A capnp capability satisfies it
through Write followed by Done, which is why a compiled node can be mounted on
a ring without any adapter beyond Consumer.
*/
type Step interface {
	Step(ctx context.Context, payload []byte) ([]byte, error)
}

/*
Consumer mounts a Step on a disruptor handler group. It reads the slots the
ring hands it, steps its target once per slot, and yields the batch onward so
the next handler group observes the same sequences.

A handler runs on the ring's own goroutine, so everything a Consumer touches
belongs to that Consumer alone. Nodes in one group run concurrently against
the same sequence; ordering between groups is the ring's barrier, never a lock
here.
*/
type Consumer struct {
	*System
	target  Step
	ring    []Slot
	mask    int64
	stepped atomic.Int64
	failed  atomic.Int64
}

func NewConsumer(ctx context.Context, name string, target Step, ring []Slot) *Consumer {
	consumer := &Consumer{
		System: NewSystem(ctx, name),
		target: target,
		ring:   ring,
		mask:   int64(len(ring) - 1),
	}

	consumer.Transition(READY)
	return consumer
}

/*
Next steps every sequence in the batch and yields the batch onward.
*/
func (consumer *Consumer) Next(batch iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	for reserved := range batch {
		if reserved == nil {
			continue
		}

		consumer.handle(*(*int64)(reserved))
	}

	return batch
}

/*
handle steps the target on the payload occupying one ring position.
*/
func (consumer *Consumer) handle(sequence int64) {
	payload := consumer.ring[sequence&consumer.mask].Payload

	if len(payload) == 0 {
		return
	}

	if _, err := consumer.target.Step(consumer.Context(), payload); err != nil {
		consumer.failed.Add(1)

		consumer.Error(errnie.Err(
			errnie.IO,
			"[runtime.consumer] step failed",
			err,
		))

		return
	}

	consumer.stepped.Add(1)
}

/*
Stepped reports how many observations this consumer has processed, and Failed
how many it could not, so a stalled stage is a number rather than a silence.
*/
func (consumer *Consumer) Stepped() int64 { return consumer.stepped.Load() }
func (consumer *Consumer) Failed() int64  { return consumer.failed.Load() }

/*
Capability steps a compiled capnp node, so anything the compiler builds can be
mounted on a ring. Write admits the observation, Done collects what the node
made of it: together they are one Step.
*/
type Capability struct {
	client      capnp.Client
	writeMethod capnp.Method
	writeSize   capnp.ObjectSize
	writeField  uint16
	doneMethod  capnp.Method
	doneSize    capnp.ObjectSize
	doneField   uint16
}

func NewCapability(
	client capnp.Client,
	writeMethod capnp.Method,
	writeSize capnp.ObjectSize,
	writeField uint16,
	doneMethod capnp.Method,
	doneSize capnp.ObjectSize,
	doneField uint16,
) *Capability {
	return &Capability{
		client:      client,
		writeMethod: writeMethod,
		writeSize:   writeSize,
		writeField:  writeField,
		doneMethod:  doneMethod,
		doneSize:    doneSize,
		doneField:   doneField,
	}
}

/*
Step admits one observation into the capability and returns what it produced.
*/
func (capability *Capability) Step(
	ctx context.Context, payload []byte,
) ([]byte, error) {
	if !capability.client.IsValid() {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"[runtime.capability] client is not valid",
			nil,
		))
	}

	send := capnp.Send{
		Method:   capability.writeMethod,
		ArgsSize: capability.writeSize,
		PlaceArgs: func(args capnp.Struct) error {
			return args.SetData(capability.writeField, payload)
		},
	}

	if err := capability.client.SendStreamCall(ctx, send); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[runtime.capability] write call failed",
			err,
		))
	}

	if err := capability.client.WaitStreaming(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[runtime.capability] streaming fence failed",
			err,
		))
	}

	answer, release := capability.client.SendCall(ctx, capnp.Send{
		Method:   capability.doneMethod,
		ArgsSize: capability.doneSize,
	})

	defer release()

	results, err := answer.Struct()

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[runtime.capability] done call failed",
			err,
		))
	}

	pointer, err := results.Ptr(capability.doneField)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.IO,
			"[runtime.capability] failed to read done result",
			err,
		))
	}

	return append([]byte(nil), pointer.Data()...), nil
}
