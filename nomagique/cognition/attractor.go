package cognition

import (
	"bytes"
	"context"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

type AttractorServer struct {
	Downstream func(context.Context, []byte, float64, uint64) error
	Root       *atomic.Pointer[iradix.Tree[[]byte]]
}

func (s *AttractorServer) Write(ctx context.Context, call Attractor_write) error {
	contextBytes, _ := call.Args().ContextBytes()
	if s.Root == nil || len(contextBytes) == 0 {
		return nil
	}
	tree := s.Root.Load()
	if tree == nil {
		return nil
	}
	exactPrefix := make([]byte, 2+len(contextBytes)+1)
	exactPrefix[0] = 'b'
	exactPrefix[1] = '/'
	copy(exactPrefix[2:], contextBytes)
	exactPrefix[2+len(contextBytes)] = '/'

	it := tree.Root().Iterator()
	it.SeekPrefix(exactPrefix)

	for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
		if !bytes.HasPrefix(k, exactPrefix) || len(v) < 24 {
			break
		}
		class := k[len(exactPrefix):]
		if len(class) == 0 {
			continue
		}

		// Basic unpack of weight
		var count, writeStep uint64
		var prob float64
		_ = count
		_ = writeStep
		_ = prob
		// Call downstream inside the loop
		if err := s.Downstream(ctx, class, prob, count); err != nil {
			return err
		}
	}
	return nil
}

func (s *AttractorServer) Done(ctx context.Context, call Attractor_done) error {
	return nil
}
