package cognition

import (
	"math"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Surprisal computes the information-theoretic surprisal in bits.
*/
type Surprisal types.Value[[]byte, float64]

/*
NewSurprisal creates a parameterless Value closure that computes the information-theoretic surprisal
(-log2 P) in bits for an observed sensory context transition based on default memory.
*/
func NewSurprisal() Surprisal {
	return NewSurprisalWithMemory(defaultMemoryRoot, defaultStepCounter)
}

/*
NewSurprisalWithMemory creates a Value closure that computes the information-theoretic surprisal
for the specified memory pointers.
*/
func NewSurprisalWithMemory(
	root *atomic.Pointer[iradix.Tree[[]byte]],
	stepCounter *atomic.Uint64,
) Surprisal {
	weightDecoder := NewWeight()

	return func(context []byte) float64 {
		if root == nil || len(context) == 0 {
			return 0
		}

		tree := root.Load()
		if tree == nil {
			return 0
		}

		var step uint64
		if stepCounter != nil {
			step = stepCounter.Load()
		}

		totalSteps := float64(step)
		if totalSteps <= 0 {
			return 0
		}

		sensoryKey := makeSensoryKey(context)
		raw, found := tree.Get(sensoryKey)
		if !found || len(raw) < 24 {
			return 0
		}

		pw := weightDecoder(raw)
		if pw == [3]uint64{} || pw[0] == 0 {
			return 0
		}

		prob := float64(pw[0]) / totalSteps
		if prob <= 0 {
			return 0
		}

		return -math.Log2(prob)
	}
}
