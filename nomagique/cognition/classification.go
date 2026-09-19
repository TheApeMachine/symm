package cognition

import (
	"iter"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
NewClassification creates a Value closure that normalizes candidate masses, identifies
the winning class via argmax, finds the runner-up, and computes confidence and evidence contrast in bits.
No structs, pure Value closure.
*/
type Classification types.Value[func(func([]byte, float64, uint64) bool), Evaluation]
func NewClassification(minContrast ...types.Float) Classification {
	return func(candidates func(func([]byte, float64, uint64) bool)) Evaluation {
		if candidates == nil {
			return nil
		}

		minCont := 0.0
		if len(minContrast) > 0 && minContrast[0] != nil {
			minCont = minContrast[0](nil)
		}

		var totalMass float64
		type cand struct {
			name    []byte
			prob    float64
			support uint64
		}
		var list []cand

		candidates(func(name []byte, prob float64, support uint64) bool {
			totalMass += prob
			list = append(list, cand{name, prob, support})
			return true
		})

		if totalMass <= 0 || len(list) == 0 {
			return nil
		}

		for i := range list {
			list[i].prob /= totalMass
		}

		winnerIdx := 0
		maxProb := list[0].prob

		for i := 1; i < len(list); i++ {
			if list[i].prob > maxProb {
				maxProb = list[i].prob
				winnerIdx = i
			}
		}

		winner := list[winnerIdx]
		var runnerUp []byte
		contrast := 0.0
		highestOther := -1.0

		for i, c := range list {
			if i != winnerIdx && c.prob > highestOther {
				highestOther = c.prob
				runnerUp = c.name
			}
		}

		if runnerUp != nil && highestOther > 0 {
			contrast = math.Log2(winner.prob / highestOther)
		}

		passed := true
		if minCont > 0 && contrast < minCont {
			passed = false
		}

		return func() (
			[]byte, []byte, float64, float64, uint64, float64, bool, float64, iter.Seq2[[]byte, float64],
		) {
			return winner.name, runnerUp, winner.prob, contrast, winner.support, 0, passed, 0, nil
		}
	}
}
