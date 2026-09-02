package probability

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
ClassSchema declares the required feature symbols and label for one class.
*/
type ClassSchema struct {
	Label   string
	Symbols []types.Symbol
}

/*
ClassifySchema returns a fail-closed classification primitive:
1. Strict Completeness: Checks if every single declared symbol is present in the Frame.
2. Logit Computation: Computes exact mean logit per class.
3. Distribution Evaluation: Runs Softmax, Argmax, and ShannonAmbiguity.
If any symbol is missing, SampleReady is set to 0 and the classifier aborts cleanly.
*/
func ClassifySchema(schemas []ClassSchema) types.Primitive {
	numClasses := len(schemas)

	return func(frame *types.Frame) {
		if numClasses == 0 || numClasses > types.MaxSamples {
			frame.Put(types.SampleReady, 0)
			return
		}

		// 1. Strict Verification: Check that ALL required symbols across all schemas exist
		for _, schema := range schemas {
			if len(schema.Symbols) == 0 {
				frame.Put(types.SampleReady, 0)
				return
			}

			for _, sym := range schema.Symbols {
				if !frame.Has(sym) {
					// Missing dimension -> fail closed (leave unready)
					frame.Put(types.SampleReady, 0)
					return
				}
			}
		}

		// 2. Score calculation: write directly into MustSampleSymbol slots
		for classIdx, schema := range schemas {
			score := 0.0
			for _, sym := range schema.Symbols {
				score += frame.MustGet(sym)
			}
			score /= float64(len(schema.Symbols))

			frame.Put(types.MustSampleSymbol(classIdx), score)
		}

		frame.Put(types.SampleCount, float64(numClasses))
		frame.Put(types.SampleReady, 1)

		// 3. Evaluate Softmax, Argmax, Confidence, and Shannon Ambiguity
		Softmax()(frame)
		Argmax()(frame)

		if winner, ok := frame.Get(SymbolWinner); ok {
			conf, _ := frame.Get(types.MustSampleSymbol(int(winner)))
			frame.Put(SymbolConfidence, conf)
		}

		ShannonAmbiguity()(frame)
	}
}
