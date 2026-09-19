package probability

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewSoftmax owns shifted exponential normalization of logits.
No structs, pure Value closure.
*/
type Softmax types.Value[[]float64, []float64]
func NewSoftmax() Softmax {
	return func(logits []float64) []float64 {
		if len(logits) == 0 {
			return nil
		}

		shift := logits[0]
		for _, logit := range logits[1:] {
			if logit > shift {
				shift = logit
			}
		}

		var total float64
		shifted := make([]float64, len(logits))
		for index, logit := range logits {
			shifted[index] = math.Exp(logit - shift)
			total += shifted[index]
		}

		if total == 0 {
			return nil
		}

		for index := range shifted {
			shifted[index] /= total
		}

		return shifted
	}
}
