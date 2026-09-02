package probability

import (
	"fmt"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolAge      = types.MustIntern("age")
	SymbolSpan     = types.MustIntern("span")
	SymbolProgress = types.MustIntern("progress")
)

/*
Classifier converts the complete generic score vector into a probability
distribution. FeatureExtractor marks complete vectors with SampleReady. Scores
are treated as logits, normalized with Softmax, and retained in the sample
slots; Winner, Confidence, and Ambiguity describe that same distribution.
*/
func Classifier(input *types.Frame) {
	ready, found := input.Get(types.SampleReady)

	if !found || ready == 0 {
		return
	}

	if ready != 1 {
		input.Err = fmt.Errorf("probability: classifier readiness must be zero or one")

		return
	}

	Softmax()(input)

	if input.Err != nil {
		return
	}

	Argmax()(input)

	if input.Err != nil {
		return
	}

	winner := int(input.MustGet(SymbolWinner))
	confidence, found := input.Get(types.MustSampleSymbol(winner))

	if !found {
		input.Err = fmt.Errorf("probability: classifier winner is absent")

		return
	}

	input.Put(SymbolConfidence, confidence)
	ShannonAmbiguity()(input)
}
