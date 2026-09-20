package ui

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"capnproto.org/go/capnp/v3"
)

type BroadcastServer struct {
	Downstream func(context.Context, capnp.Ptr) error
}

func NewBroadcastServer() *BroadcastServer {
	return &BroadcastServer{}
}

func (s *BroadcastServer) Write(ctx context.Context, call Broadcast_write) error {
	args, err := call.Args().Broadcast()
	if err != nil {
		// fallback to see if it's named something else
		return err
	}
	
	payloadPtr, err := args.Payload()
	if err != nil {
		return err
	}
	
	var out capnp.Ptr
	if payloadPtr.IsValid() {
		out = payloadPtr
	}
	if s.Downstream != nil {
		return s.Downstream(ctx, out)
	}
	return nil
}

func (s *BroadcastServer) Done(ctx context.Context, call Broadcast_done) error {
	return nil
}



type BroadcastNode types.StreamNode[any, any]

func NewBroadcast() BroadcastNode {
	server := &BroadcastServer{}
	var nativeDownstream func(context.Context, any) error
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			if nativeDownstream != nil {
				return nativeDownstream(ctx, payload)
			}
			return nil
		},
		func(next func(context.Context, any) error) {
			nativeDownstream = next
		},
	)
}
