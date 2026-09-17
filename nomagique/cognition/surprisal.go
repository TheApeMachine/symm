package cognition

import (
	"iter"
	"math"
	"sync/atomic"
	"unsafe"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Surprisal computes the information-theoretic surprisal (-log2 P) in bits
for an observed sensory context transition based on its measured occurrence frequency.
*/
type Surprisal struct {
	*core.PrimitiveError
	root        *atomic.Pointer[iradix.Tree[[]byte]]
	stepCounter *atomic.Uint64
	weight      *Weight
	out         float64
}

func NewSurprisal(
	root *atomic.Pointer[iradix.Tree[[]byte]],
	stepCounter *atomic.Uint64,
) *Surprisal {
	return &Surprisal{
		PrimitiveError: core.NewPrimitiveError(),
		root:           root,
		stepCounter:    stepCounter,
		weight:         NewWeight(),
	}
}

func (surprisal *Surprisal) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if surprisal.Error() != nil || surprisal.root == nil {
			return
		}

		tree := surprisal.root.Load()
		if tree == nil {
			return
		}

		var step uint64
		if surprisal.stepCounter != nil {
			step = surprisal.stepCounter.Load()
		}

		totalSteps := float64(step)

		for arriving := range in {
			if arriving == nil {
				continue
			}

			context := *(*[]byte)(arriving)
			if len(context) == 0 {
				continue
			}

			sensoryKey := makeSensoryKey(context)
			raw, found := tree.Get(sensoryKey)
			if !found || len(raw) < WeightSize || totalSteps <= 0 {
				continue
			}

			var pw PackedWeight
			inW := func(yieldW func(unsafe.Pointer) bool) {
				yieldW(unsafe.Pointer(&raw))
			}
			for outW := range surprisal.weight.Next(inW) {
				pw = *(*PackedWeight)(outW)
			}

			if pw.Count == 0 {
				continue
			}

			prob := float64(pw.Count) / totalSteps
			if prob <= 0 {
				continue
			}

			surprisal.out = -math.Log2(prob)
			if !yield(unsafe.Pointer(&surprisal.out)) {
				return
			}
		}
	}
}
