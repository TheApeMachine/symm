package runtime

import (
	"bytes"
	"context"

	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/*
	advance publishes a source cycle and joins its native completion barrier.

The preceding source results carry configuration, such as discovered symbols,
to the next cycle. A source never reads a concurrently changing peer result.
*/
func (server *WorkspaceServer) advance(ctx context.Context) error {
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return errnie.Error(err)
	}
	defer message.Release()
	feedback, err := NewRootCompletion(segment)

	if err != nil {
		return errnie.Error(err)
	}

	if server.published > 0 {
		previous := server.slots[(server.published-1)&server.mask]

		for _, result := range previous.Results[:server.sources] {
			if err := feedback.Append(result); err != nil {
				return err
			}
		}
	}
	upstream, err := feedback.Outputs()

	if err != nil {
		return errnie.Error(err)
	}
	reserved, err := server.publish(ctx, nil, server.epoch, server.published, upstream)

	if err != nil {
		return err
	}

	if err := server.drain(ctx, reserved+1); err != nil {
		return err
	}
	server.data = nil

	for _, result := range server.slots[reserved&server.mask].Results[:server.sources] {
		payload, err := result.Data()

		if err != nil {
			return errnie.Error(err)
		}

		if len(payload) > 0 {
			server.data = bytes.Clone(payload)
		}
	}
	return nil
}

/*
	consume completes every source observation in this ring slot before returning

to the native listener. Its group barrier is still owned solely by LMAX.
*/
func (server *WorkspaceServer) consume(consumer Consumer, slot Slot, previous, position int) (Completion, error) {
	if !server.sourceMode || previous == 0 || server.sources == 1 {
		return server.invoke(consumer, slot, previous, position, 0)
	}
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return Completion{}, errnie.Error(err)
	}
	combined, err := NewRootCompletion(segment)

	if err != nil {
		message.Release()
		return Completion{}, errnie.Error(err)
	}

	for observation := range server.sources {
		result, err := server.invoke(consumer, slot, previous, position, observation)

		if err != nil {
			message.Release()
			return Completion{}, err
		}
		err = combined.Append(result)

		result.Message().Release()

		if err != nil {
			message.Release()
			return Completion{}, errnie.Error(err)
		}
	}
	return combined, nil
}

/* invoke preserves each feed's distinct observation stamp across every group. */
func (server *WorkspaceServer) invoke(consumer Consumer, slot Slot, previous, position, observation int) (Completion, error) {
	future, release := consumer.Step(server.Context(), func(args StageNode_step_Params) error {
		sequence, payload := slot.Sequence, slot.Payload

		if server.sourceMode {
			ordinal := position

			if previous > 0 {
				ordinal = observation
				var err error
				payload, err = slot.Results[observation].Data()

				if err != nil {
					return errnie.Error(err)
				}
			}
			sequence = slot.Sequence*int64(server.sources) + int64(ordinal)
		}
		args.SetEpoch(slot.Epoch)
		args.SetSequence(sequence)

		if err := server.upstream(args, slot, previous); err != nil {
			return err
		}
		return args.SetData(payload)
	})
	defer release()
	result, err := future.Struct()

	if err != nil {
		return Completion{}, errnie.Error(err)
	}
	return result.Clone()
}
