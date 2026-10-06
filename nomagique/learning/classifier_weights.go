package learning

import (
	"fmt"
	"iter"
	"math"
	"unsafe"

	"github.com/theapemachine/symm/nomagique/core"
)

/*
ClassifierWeights holds dynamically derived coefficients for configured
outputs and scores feature maps against them.

Each output is a balanced combination of its terms: term weights are the
inverse feature scales, normalized to sum to one. Each feature is scaled and
squashed to (-1, 1); an inverted term contributes one minus that value.

Each arrival is *map[string]float64 (the features); it yields *[]float64 with
one logit per configured output, in order. The first output is the strength.
A rejected configuration is recorded at construction and every stream over it
yields nothing.
*/
type ClassifierWeights struct {
	*core.PrimitiveError
	threshold   float64
	scales      map[string]float64
	outputs     []string
	terms       map[string][]string
	inverts     map[string]map[string]bool
	termWeights map[string]map[string]float64
	out         []float64
}

func NewClassifierWeights(
	threshold float64,
	scales map[string]float64,
	outputs []string,
	terms map[string][]string,
	inverts map[string][]string,
) core.Primitive {
	weights := &ClassifierWeights{
		PrimitiveError: core.NewPrimitiveError(),
		threshold:      threshold,
		scales:         make(map[string]float64, len(scales)),
		outputs:        append([]string(nil), outputs...),
		terms:          make(map[string][]string, len(outputs)),
		inverts:        make(map[string]map[string]bool, len(outputs)),
		termWeights:    make(map[string]map[string]float64, len(outputs)),
		out:            make([]float64, len(outputs)),
	}

	for key, value := range scales {
		weights.scales[key] = value
	}

	if !(threshold > 0) {
		weights.Error(fmt.Errorf(
			"%w: classifier weights threshold must be positive, got %v",
			core.ErrDomain, threshold,
		))
		return weights
	}

	if len(outputs) == 0 {
		weights.Error(fmt.Errorf("%w: classifier weights require outputs", core.ErrShape))
		return weights
	}

	for _, outputKey := range outputs {
		outputTerms := terms[outputKey]

		if len(outputTerms) == 0 {
			weights.Error(fmt.Errorf("%w: output %q requires terms", core.ErrShape, outputKey))
			return weights
		}

		raw := make(map[string]float64, len(outputTerms))
		total := 0.0

		for _, featureKey := range outputTerms {
			scale := scales[featureKey]

			if !(scale > 0) || math.IsInf(scale, 0) {
				weights.Error(fmt.Errorf(
					"%w: feature scale for %s must be finite and positive, got %v",
					core.ErrDomain, featureKey, scale,
				))
				return weights
			}

			raw[featureKey] = 1.0 / scale
			total += raw[featureKey]
		}

		for featureKey := range raw {
			raw[featureKey] /= total
		}

		inverted := make(map[string]bool, len(inverts[outputKey]))

		for _, featureKey := range inverts[outputKey] {
			inverted[featureKey] = true
		}

		weights.terms[outputKey] = append([]string(nil), outputTerms...)
		weights.inverts[outputKey] = inverted
		weights.termWeights[outputKey] = raw
	}

	return weights
}

func (op *ClassifierWeights) Next(in iter.Seq[unsafe.Pointer]) iter.Seq[unsafe.Pointer] {
	if op.Error() != nil {
		return func(yield func(unsafe.Pointer) bool) {}
	}

	return func(yield func(unsafe.Pointer) bool) {
		for arriving := range in {
			if arriving == nil {
				op.Error(core.ErrShape)
				return
			}

			features := *(*map[string]float64)(arriving)
			op.out = make([]float64, len(op.outputs))

			for index, outputKey := range op.outputs {
				score := 0.0

				for _, featureKey := range op.terms[outputKey] {
					ratio := features[featureKey] / op.scales[featureKey]
					normalized := math.NaN()

					if !math.IsNaN(ratio) && !math.IsInf(ratio, 0) {
						normalized = ratio / (1.0 + math.Abs(ratio))
					}

					if op.inverts[outputKey][featureKey] {
						normalized = 1.0 - normalized
					}

					score += normalized * op.termWeights[outputKey][featureKey]
				}

				op.out[index] = score
			}

			if !yield(unsafe.Pointer(&op.out)) {
				return
			}
		}
	}
}
