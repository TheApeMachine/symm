package runtime

import (
	capnp "capnproto.org/go/capnp/v3"
	"github.com/theapemachine/errnie"
)

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
