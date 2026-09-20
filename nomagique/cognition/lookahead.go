package cognition

import (
	"bytes"
	"context"
	"math"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/errnie"
)

type LookaheadServer struct {
	Root *atomic.Pointer[iradix.Tree[[]byte]]
	seq  []byte
	logP float64
}

func NewLookahead() *LookaheadServer {
	return &LookaheadServer{}
}

func (s *LookaheadServer) Write(ctx context.Context, call Lookahead_write) error {
	prefix, _ := call.Args().Prefix()
	if s.Root == nil || len(prefix) == 0 {
		return nil
	}

	tree := s.Root.Load()
	if tree == nil {
		return nil
	}

	searchPrefix := append([]byte("s/"), prefix...)
	it := tree.Root().Iterator()
	it.SeekPrefix(searchPrefix)

	for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
		if !bytes.HasPrefix(k, searchPrefix) || len(v) < 24 {
			break
		}
		seq := k[len("s/"):]
		if len(seq) <= len(prefix) {
			continue
		}

		s.seq = bytes.Clone(seq)
		s.logP = math.Log(1.0)
		break
	}
	return nil
}

func (s *LookaheadServer) Done(ctx context.Context, call Lookahead_done) error {
	results, err := call.AllocResults()
	if err != nil {
		return errnie.Error(errnie.Err(errnie.Internal, "failed to alloc results", err))
	}

	_ = results.SetSeq(s.seq)
	results.SetLogP(s.logP)

	s.seq = nil
	s.logP = 0
	return nil
}
