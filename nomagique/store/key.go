package store

import (
	"github.com/theapemachine/symm/nomagique/types"
	"context"
)

// KeyServer implements Key_Server from the capnp schema.
type KeyServer struct {
	path []string
}

func NewKeyServer(path ...string) *KeyServer {
	return &KeyServer{
		path: path,
	}
}

func (s *KeyServer) Extract(ctx context.Context, call Key_extract) error {
	// Dynamically extracting values by string path from an AnyPointer 
	// requires Cap'n Proto dynamic schema introspection.
	// For now, we will return not found, until dynamic schema is fully enabled.

	res, err := call.AllocResults()
	if err != nil {
		return err
	}

	res.SetFound(false)
	res.SetValue(0.0)

	return nil
}



type KeyNode types.StreamNode[any, any]

func NewKey() KeyNode {
	server := &KeyServer{}
	return types.NewStreamNode(
		server,
		func(ctx context.Context, payload any) error {
			return nil
		},
		func(next func(context.Context, any) error) {},
	)
}
