package store

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
	"sync/atomic"

	capnp "capnproto.org/go/capnp/v3"
	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

// RadixServer implements Radix_Server from the capnp schema.
type RadixServer struct {
	root atomic.Pointer[iradix.Tree[[]byte]]
}

func NewRadixServer() *RadixServer {
	s := &RadixServer{}
	s.root.Store(iradix.New[[]byte]())
	return s
}

func (s *RadixServer) Read(ctx context.Context, call Radix_read) error {
	key, err := call.Args().Key()
	if err != nil {
		return err
	}

	current := s.root.Load()
	val, found := current.Get(key)

	res, err := call.AllocResults()
	if err != nil {
		return err
	}
	res.SetFound(found)

	if found {
		// val is []byte containing a Cap'n Proto serialized message
		msg, err := capnp.Unmarshal(val)
		if err == nil {
			rootPtr, err := msg.Root()
			if err == nil {
				res.SetValue(rootPtr)
			}
		}
	}

	return nil
}

func (s *RadixServer) Write(ctx context.Context, call Radix_write) error {
	key, err := call.Args().Key()
	if err != nil {
		return err
	}
	
	valPtr, err := call.Args().Value()
	if err != nil {
		return err
	}
	
	// Serialize valPtr into []byte to store safely
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err == nil {
		err = msg.SetRoot(valPtr)
		if err == nil {
			bytes, err := seg.Message().Marshal()
			if err == nil {
				current := s.root.Load()
				updated, _, _ := current.Insert(key, bytes)
				s.root.Store(updated)
				
				res, err := call.AllocResults()
				if err == nil {
					res.SetValue(valPtr)
				}
			}
		}
	}

	return nil
}

func (s *RadixServer) Identify(ctx context.Context, call Radix_identify) error {
	key, err := call.Args().Key()
	if err != nil {
		return err
	}
	
	valPtr, err := call.Args().Value()
	if err != nil {
		return err
	}

	current := s.root.Load()
	val, found := current.Get(key)

	res, err := call.AllocResults()
	if err != nil {
		return err
	}

	if found {
		msg, err := capnp.Unmarshal(val)
		if err == nil {
			rootPtr, err := msg.Root()
			if err == nil {
				res.SetValue(rootPtr)
			}
		}
		return nil
	}

	// Insert if not found
	msg, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err == nil {
		err = msg.SetRoot(valPtr)
		if err == nil {
			bytes, err := seg.Message().Marshal()
			if err == nil {
				updated, _, _ := current.Insert(key, bytes)
				s.root.Store(updated)
				res.SetValue(valPtr)
			}
		}
	}

	return nil
}



type RadixNode types.StreamNode[any, any]

func NewRadix() RadixNode {
	server := &RadixServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
