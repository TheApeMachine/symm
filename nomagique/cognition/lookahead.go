package cognition

import (
	"bytes"
	"iter"
	"math"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Lookahead explores forward continuation paths in the sensory tree.
It follows only empirical branches observed in the store, scoring paths
by cumulative log-probability until branches terminate.
*/
type Lookahead struct {
	*core.PrimitiveError
	root   *atomic.Pointer[iradix.Tree[[]byte]]
	weight *Weight
	out    LookaheadPath
}

func NewLookahead(
	root *atomic.Pointer[iradix.Tree[[]byte]],
) *Lookahead {
	return &Lookahead{
		PrimitiveError: core.NewPrimitiveError(),
		root:           root,
		weight:         NewWeight(),
	}
}

func (lookahead *Lookahead) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if lookahead.Error() != nil || lookahead.root == nil {
			return
		}

		tree := lookahead.root.Load()
		if tree == nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				continue
			}

			prefix := *(*[]byte)(arriving)
			if len(prefix) == 0 {
				continue
			}

			searchPrefix := append([]byte("s/"), prefix...)
			it := tree.Root().Iterator()
			it.SeekPrefix(searchPrefix)

			for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
				if !bytes.HasPrefix(k, searchPrefix) || len(v) < WeightSize {
					break
				}

				seq := k[len("s/"):]
				if len(seq) <= len(prefix) {
					continue
				}

				var pw PackedWeight
				inW := func(yieldW func(unsafe.Pointer) bool) {
					yieldW(unsafe.Pointer(&v))
				}
				for outW := range lookahead.weight.Next(inW) {
					pw = *(*PackedWeight)(outW)
				}

				if pw.Count == 0 || pw.Mass <= 0 {
					continue
				}

				logP := math.Log(pw.Mass)

				lookahead.out = LookaheadPath{
					Sequence: string(seq),
					Score:    logP,
				}

				if !yield(unsafe.Pointer(&lookahead.out)) {
					return
				}
			}
		}
	}
}
