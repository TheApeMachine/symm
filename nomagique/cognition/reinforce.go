package cognition

import (
	"bytes"
	"context"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

type ReinforceServer struct {
	Downstream  func(context.Context, []byte) error
	Root        *atomic.Pointer[iradix.Tree[[]byte]]
	StepCounter *atomic.Uint64
}

func (s *ReinforceServer) Write(ctx context.Context, call Reinforce_write) error {
	contextBytes, _ := call.Args().ContextBytes()
	classBytes, _ := call.Args().ClassBytes()
	// TODO: implement full iradix CAS reinforcement loop
	if len(classBytes) > 0 {
		return s.Downstream(ctx, bytes.Clone(classBytes))
	}
	return s.Downstream(ctx, bytes.Clone(contextBytes))
}

func (s *ReinforceServer) Done(ctx context.Context, call Reinforce_done) error {
	return nil
}
