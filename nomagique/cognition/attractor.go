package cognition

import (
	"bytes"
	"iter"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Attractor seeks matching attractor basin records in the radix trie under b/<context>/.
It yields observed class candidates with empirical evidence mass and support count.
Unseen contexts produce no candidates; missing evidence stays missing.
*/
type Attractor struct {
	*core.PrimitiveError
	root   *atomic.Pointer[iradix.Tree[[]byte]]
	weight *Weight
	out    ClassCandidate
}

func NewAttractor(
	root *atomic.Pointer[iradix.Tree[[]byte]],
) *Attractor {
	return &Attractor{
		PrimitiveError: core.NewPrimitiveError(),
		root:           root,
		weight:         NewWeight(),
	}
}

func (attractor *Attractor) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if attractor.Error() != nil || attractor.root == nil {
			return
		}

		tree := attractor.root.Load()
		if tree == nil {
			return
		}

		for arriving := range in {
			if arriving == nil {
				continue
			}

			context := *(*[]byte)(arriving)
			if len(context) == 0 {
				continue
			}

			exactPrefix := make([]byte, 2+len(context)+1)
			exactPrefix[0] = 'b'
			exactPrefix[1] = '/'
			copy(exactPrefix[2:], context)
			exactPrefix[2+len(context)] = '/'

			it := tree.Root().Iterator()
			it.SeekPrefix(exactPrefix)

			for k, v, ok := it.Next(); ok; k, v, ok = it.Next() {
				if !bytes.HasPrefix(k, exactPrefix) || len(v) < WeightSize {
					break
				}

				class := k[len(exactPrefix):]
				if len(class) == 0 {
					continue
				}

				var pw PackedWeight
				inWeight := func(yieldW func(unsafe.Pointer) bool) {
					yieldW(unsafe.Pointer(&v))
				}
				for outW := range attractor.weight.Next(inWeight) {
					pw = *(*PackedWeight)(outW)
				}

				attractor.out = ClassCandidate{
					Name:        string(class),
					Probability: pw.Mass,
					Support:     pw.Count,
				}

				if !yield(unsafe.Pointer(&attractor.out)) {
					return
				}
			}
		}
	}
}
