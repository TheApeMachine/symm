package cognition

import (
	"bytes"
	"context"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
)

type AttractorServer struct {
	Root  *atomic.Pointer[iradix.Tree[[]byte]]
	class []byte
	prob  float64
	count uint64
}

func NewAttractor() *AttractorServer {
	return &AttractorServer{}
}

func (s *AttractorServer) Write(ctx context.Context, call Attractor_write) error {
	contextBytes, err := call.Args().ContextBytes()
	if err != nil || s.Root == nil || len(contextBytes) == 0 {
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

		s.class = bytes.Clone(class)
		s.prob = 1.0
		s.count = 1
		break
	}
	return nil
}

func (s *AttractorServer) Done(ctx context.Context, call Attractor_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	_ = results.SetClass(s.class)
	results.SetProb(s.prob)
	results.SetCount(s.count)

	s.class = nil
	s.prob = 0
	s.count = 0
	return nil
}
