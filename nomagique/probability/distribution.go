package probability

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Softmax normalizes a collection of logits into a probability simplex.

It is a pure Reduction-tier fold returning the winning class's probability.
The shift by the observed maximum before exponentiating is the standard
numerically-stable formulation — an algebraic identity on the result, not a
tuning constant.

An empty collection, or one carrying a non-finite logit, has no distribution
and reduces to 0.
*/
var Softmax types.Reduction = func(logits []types.Number) types.Number {
	if len(logits) == 0 {
		return 0
	}

	maximum := math.Inf(-1)

	for _, logit := range logits {
		value := float64(logit)

		if math.IsNaN(value) || math.IsInf(value, 0) {
			return 0
		}

		if value > maximum {
			maximum = value
		}
	}

	total := 0.0
	best := 0.0

	for _, logit := range logits {
		weight := math.Exp(float64(logit) - maximum)
		total += weight

		if weight > best {
			best = weight
		}
	}

	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		return 0
	}

	return types.Number(best / total)
}

/*
Distribution normalizes logits into a probability simplex and reports the
winner, its confidence, and the distribution's ambiguity.

Logits are supplied through the Logits slot, a node collection whose Values
the distribution folds. Step returns the winning class's probability; the
winner index, ambiguity and the full simplex are exposed as accessors per
the multi-output Equation Encapsulation rule.

Degenerate behavior: an omitted Logits slot has nothing to normalize and
yields 0.
*/
type Distribution struct {
	Logits Collection

	probabilities []types.Number
	winner        int
	confidence    types.Number
	ambiguity     types.Number
	ready         bool
}

/*
Collection supplies a variable-width vector to a node that folds it. It is
the bridge between the unary carrier and reductions that need many values.
*/
type Collection interface {
	Values() []types.Number
}

func (dist *Distribution) Step(types.Number) types.Number {
	if dist.Logits == nil {
		dist.reset(0)

		return 0
	}

	logits := dist.Logits.Values()

	dist.reset(len(logits))

	if len(logits) == 0 {
		return 0
	}

	maximum := math.Inf(-1)

	for _, logit := range logits {
		value := float64(logit)

		if math.IsNaN(value) || math.IsInf(value, 0) {
			dist.reset(len(logits))

			return 0
		}

		if value > maximum {
			maximum = value
		}
	}

	total := 0.0

	for index, logit := range logits {
		weight := math.Exp(float64(logit) - maximum)
		dist.probabilities[index] = types.Number(weight)
		total += weight
	}

	if total <= 0 || math.IsNaN(total) || math.IsInf(total, 0) {
		dist.reset(len(logits))

		return 0
	}

	entropy := 0.0

	for index := range dist.probabilities {
		probability := dist.probabilities[index] / types.Number(total)
		dist.probabilities[index] = probability

		if probability > dist.confidence {
			dist.confidence = probability
			dist.winner = index
		}

		if probability > 0 {
			entropy -= float64(probability) * math.Log(float64(probability))
		}
	}

	// Normalized by the entropy of the uniform distribution over exactly the
	// classes present, so ambiguity is comparable across class counts.
	if len(logits) > 1 {
		dist.ambiguity = types.Number(entropy / math.Log(float64(len(logits))))
	}

	dist.ready = true

	return dist.confidence
}

func (dist *Distribution) reset(size int) {
	if cap(dist.probabilities) >= size {
		dist.probabilities = dist.probabilities[:size]
	} else {
		dist.probabilities = make([]types.Number, size)
	}

	for index := range dist.probabilities {
		dist.probabilities[index] = 0
	}

	dist.winner = 0
	dist.confidence = 0
	dist.ambiguity = 0
	dist.ready = false
}

// Ready reports whether the last step produced a defined distribution.
func (dist *Distribution) Ready() bool { return dist.ready }

// Winner returns the index of the highest-probability class.
func (dist *Distribution) Winner() int { return dist.winner }

// Confidence returns the winning class's probability.
func (dist *Distribution) Confidence() types.Number { return dist.confidence }

/*
Ambiguity returns the Shannon entropy normalized to [0, 1]: zero when one
class holds all the mass, one when it is spread uniformly.
*/
func (dist *Distribution) Ambiguity() types.Number { return dist.ambiguity }

// Sharpness is the complement of Ambiguity.
func (dist *Distribution) Sharpness() types.Number { return 1 - dist.ambiguity }

/*
Values exposes the simplex, so a Distribution itself satisfies Collection and
composes into another folding node.
*/
func (dist *Distribution) Values() []types.Number { return dist.probabilities }

/*
Probability returns one class's mass, or 0 outside the distribution.
*/
func (dist *Distribution) Probability(index int) types.Number {
	if index < 0 || index >= len(dist.probabilities) {
		return 0
	}

	return dist.probabilities[index]
}

var (
	_ types.Node = (*Distribution)(nil)
	_ Collection = (*Distribution)(nil)
)
