package cognition

import (
	"math"
	"sync/atomic"

	iradix "github.com/hashicorp/go-immutable-radix/v2"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewSurprisal creates a Value closure that computes the information-theoretic surprisal
(-log2 P) in bits for an observed sensory context transition based on its measured occurrence frequency.
No structs, pure Value closure.
*/
func NewSurprisal(
	root *atomic.Pointer[iradix.Tree[[]byte]],
	stepCounter *atomic.Uint64,
) types.Value[[]byte, float64] {
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
