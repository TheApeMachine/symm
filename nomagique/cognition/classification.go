package cognition

import (
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/probability"
)

/*
ClassificationResult carries the normalized attractor classification readout.
*/
type ClassificationResult struct {
	WinnerClass string
	RunnerUp    string
	Confidence  float64
	Contrast    float64
	Support     uint64
	IsTie       bool
	Candidates  []ClassCandidate
}

/*
Classification normalizes candidate masses, identifies the winning class via
Argmax, finds the runner-up, and computes confidence and evidence contrast in bits.
*/
type Classification struct {
	*core.PrimitiveError
	out ClassificationResult
}

func NewClassification() *Classification {
	return &Classification{
		PrimitiveError: core.NewPrimitiveError(),
	}
}

func (classification *Classification) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	return func(yield func(unsafe.Pointer) bool) {
		if classification.Error() != nil {
			return
		}

		var candidates []ClassCandidate
		var totalMass float64

		for arriving := range in {
			if arriving == nil {
				continue
			}

			c := *(*ClassCandidate)(arriving)
			candidates = append(candidates, c)
			totalMass += c.Probability
		}

		if len(candidates) == 0 || totalMass <= 0 {
			return
		}

		for i := range candidates {
			candidates[i].Probability /= totalMass
		}

		argmax := probability.NewArgmax()
		argmaxIn := func(yieldVal func(unsafe.Pointer) bool) {
			for i := range candidates {
				val := candidates[i].Probability
				if !yieldVal(unsafe.Pointer(&val)) {
					return
				}
			}
		}

		winnerIdx := 0
		for out := range argmax.Next(argmaxIn) {
			res := (*probability.ArgmaxResult)(out)
			winnerIdx = res.Index
		}

		winner := candidates[winnerIdx]
		runnerUp := ""
		contrast := 0.0
		highestOther := -1.0

		for i, c := range candidates {
			if i != winnerIdx && c.Probability > highestOther {
				highestOther = c.Probability
				runnerUp = c.Name
			}
		}

		if runnerUp != "" && highestOther > 0 {
			contrast = math.Log2(winner.Probability / highestOther)
		}

		isTie := len(candidates) > 1 && (winner.Probability == highestOther || contrast == 0.0)

		classification.out = ClassificationResult{
			WinnerClass: winner.Name,
			RunnerUp:    runnerUp,
			Confidence:  winner.Probability,
			Contrast:    contrast,
			Support:     winner.Support,
			IsTie:       isTie,
			Candidates:  candidates,
		}

		yield(unsafe.Pointer(&classification.out))
	}
}
