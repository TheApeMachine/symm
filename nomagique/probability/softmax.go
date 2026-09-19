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

func NewSoftmax(logits ...types.Float) Softmax {
	return func(in []float64) []float64 {
		l := in
		if len(logits) > 0 {
			l = make([]float64, len(logits))
			for i, lg := range logits {
				if lg != nil {
					l[i] = lg(in)
				}
			}
		}

		if len(l) == 0 {
			return nil
		}

		shift := l[0]
		for _, logit := range l[1:] {
			if logit > shift {
				shift = logit
			}
		}

		var total float64
		shifted := make([]float64, len(l))
		for index, logit := range l {
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
