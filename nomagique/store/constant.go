package store

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"

	capnp "capnproto.org/go/capnp/v3"
)

// ConstantServer implements Constant_Server from the capnp schema.
type ConstantServer struct {
	value []byte
}

func NewConstantServer(val capnp.Ptr) *ConstantServer {
	s := &ConstantServer{}
	if val.IsValid() {
		msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
		if err == nil {
			if msg.SetRoot(val) == nil {
				if bytes, err := seg.Message().Marshal(); err == nil {
					s.value = bytes
				}
			}
		}
	}
	return s
}

func (s *ConstantServer) Evaluate(ctx context.Context, call Constant_evaluate) error {
	res, err := call.AllocResults()
	if err != nil {
		return err
	}

	if len(s.value) > 0 {
		msg, err := capnp.Unmarshal(s.value)
		if err == nil {
			rootPtr, err := msg.Root()
			if err == nil {
				res.SetValue(rootPtr)
			}
		}
	}

	return nil
}



type ConstantNode types.StreamNode[any, any]

func NewConstant() ConstantNode {
	server := &ConstantServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
