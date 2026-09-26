package runtime

import (
	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

/* Append preserves every typed result when a ring cycle carries several observations. */
func (completion Completion) Append(other Completion) error {
	current, err := completion.Outputs()

	if err != nil {
		return errnie.Error(err)
	}
	incoming, err := other.Outputs()

	if err != nil {
		return errnie.Error(err)
	}
	outputs, err := completion.NewOutputs(int32(current.Len() + incoming.Len()))

	if err != nil {
		return errnie.Error(err)
	}
	position := 0

	for _, values := range []Result_List{current, incoming} {
		for index := range values.Len() {
			if err := outputs.Set(position, values.At(index)); err != nil {
				return errnie.Error(err)
			}
			position++
		}
	}
	return nil
}

/* Clone gives a completed observation ownership independent of its RPC lifetime. */
func (completion Completion) Clone() (Completion, error) {
	message, segment, err := capnp.NewMessage(capnp.SingleSegment(nil))

	if err != nil {
		return Completion{}, errnie.Error(errnie.Err(errnie.Internal, "completion: allocate retained message", err))
	}
	retained, err := NewRootCompletion(segment)

	if err == nil {
		err = capnp.Struct(retained).CopyFrom(capnp.Struct(completion))
	}

	if err != nil {
		message.Release()
		return Completion{}, errnie.Error(errnie.Err(errnie.Internal, "completion: retain results", err))
	}
	return retained, nil
}
