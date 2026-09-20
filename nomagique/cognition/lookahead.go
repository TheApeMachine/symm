package cognition

import (
	"bytes"
	"context"
	"math"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
)

type LookaheadServer struct {
	Downstream func(context.Context, []byte, float64) error
	Root       *atomic.Pointer[iradix.Tree[[]byte]]
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
		// TODO: decode weight
		logP := math.Log(1.0)
		if err := s.Downstream(ctx, seq, logP); err != nil {
			return err
		}
	}
	return nil
}

func (s *LookaheadServer) Done(ctx context.Context, call Lookahead_done) error {
	return nil
}
