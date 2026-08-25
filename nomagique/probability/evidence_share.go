package probability

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

var (
	// SymbolWinner is the sample index of the strongest category.
	SymbolWinner = types.MustIntern("probability/winner")
	// SymbolConfidence is the winner's symmetric-one-pseudocount evidence share.
	SymbolConfidence = types.MustIntern("probability/confidence")
	// SymbolAmbiguity is the normalized Shannon entropy of the distribution.
	SymbolAmbiguity = types.MustIntern("probability/ambiguity")
)

/*
Argmax returns a Primitive that selects the index of the strongest present
category strength across the generic sample slots. It is the atomic selection
unit: it reads every populated sample/N slot, resolves the maximum (ties to the
lowest index, deterministically), and writes the winning index into
SymbolWinner. It is independent of how strengths were produced or how they will
be consumed, so it composes with EvidenceShare and any other consumer.
*/
func Argmax() types.Primitive {
	return func(input types.Frame) types.Frame {
		bestIndex := 0
		bestValue := 0.0
		found := false

		for index := range types.MaxSamples {
			value, present := input.Get(types.MustSampleSymbol(index))

			if !present {
				continue
			}

			if !finitePositive(value) && value != 0 {
				input.Err = fmt.Errorf(
					"probability: argmax sample/%d must be finite and non-negative",
					index,
				)

				return input
			}

			if !found || value > bestValue {
				bestValue = value
				bestIndex = index
				found = true
			}
		}

		if !found {
			input.Err = fmt.Errorf("probability: argmax requires at least one sample")

			return input
		}

		input.Put(SymbolWinner, float64(bestIndex))

		return input
	}
}

/*
EvidenceShare returns a Primitive that computes a category's symmetric
one-pseudocount evidence-share confidence

	P_i = (S_i + 1) / (sum_j S_j + K)

over the K present strengths in the generic sample slots. The selected slot is
the winning index written by Argmax; when absent, the maximum-strength slot is
used. Every category contributes one unit of pseudocount, so K equal strengths
yield 1/K, a near-tie stays near 1/K, and a lone category cannot reach 1.0 from
its own finite evidence alone. Common power-of-two scaling keeps the ratio exact
under unbounded positive evidence. The winner index and the confidence are
written back to SymbolWinner and SymbolConfidence.
*/
func EvidenceShare() types.Primitive {
	return func(input types.Frame) types.Frame {
		strengths, _ := collectSamples(input)

		if len(strengths) == 0 {
			input.Err = fmt.Errorf("probability: evidence share requires samples")

			return input
		}

		selected := 0

		if index, hasWinner := input.Get(SymbolWinner); hasWinner {
			selected = int(index)
		} else {
			selected = argmaxIndex(strengths)
		}

		if selected < 0 || selected >= len(strengths) {
			input.Err = fmt.Errorf("probability: evidence share winner index out of range")

			return input
		}

		selectedStrength := strengths[selected]

		if selectedStrength <= 0 {
			for _, strength := range strengths {
				if strength > 0 {
					input.Err = fmt.Errorf(
						"probability: evidence share requires positive selected evidence",
					)

					return input
				}
			}

			input.Put(SymbolWinner, float64(selected))
			input.Put(SymbolConfidence, 1.0/float64(len(strengths)))

			return input
		}

		exponent := evidenceExponent(strengths)
		evidenceSum := 0.0

		for _, strength := range strengths {
			if strength > 0 {
				evidenceSum += math.Ldexp(strength, -exponent)
			}
		}

		pseudocount := math.Ldexp(1, -exponent)
		numerator := math.Ldexp(selectedStrength, -exponent) + pseudocount
		denominator := evidenceSum + float64(len(strengths))*pseudocount

		input.Put(SymbolWinner, float64(selected))
		input.Put(SymbolConfidence, numerator/denominator)

		return input
	}
}

/*
ShannonAmbiguity returns a Primitive that computes the normalized Shannon
entropy U = H / log2(K) over the K present values in the generic sample slots,
treated as a probability distribution. Low U means the distribution concentrates
on few regimes; high U means the evidence does not distinguish them. It is a
distribution-level quantity and is not 1 - Confidence. The result is written to
SymbolAmbiguity. The input need not be normalized; the primitive normalizes the
present values into a probability distribution before measuring entropy, so it
composes with a bare strength vector or a confidence vector alike.
*/
func ShannonAmbiguity() types.Primitive {
	return func(input types.Frame) types.Frame {
		values, found := collectSamples(input)

		if len(values) == 0 {
			input.Err = fmt.Errorf("probability: shannon ambiguity requires samples")

			return input
		}

		total := 0.0

		for _, value := range values {
			if value < 0 {
				input.Err = fmt.Errorf("probability: shannon ambiguity requires non-negative values")

				return input
			}

			total += value
		}

		if total <= 0 {
			input.Put(SymbolAmbiguity, 1.0)

			return input
		}

		entropy := 0.0

		for _, value := range values {
			if value <= 0 {
				continue
			}

			probability := value / total
			entropy -= probability * math.Log2(probability)
		}

		maximum := math.Log2(float64(len(values)))

		if maximum <= 0 {
			input.Put(SymbolAmbiguity, 0)

			return input
		}

		ambiguity := entropy / maximum

		input.Put(SymbolAmbiguity, clampUnit(ambiguity))

		_ = found

		return input
	}
}

/*
collectSamples collects every populated generic sample slot into a slice in
ascending slot order, rejecting non-finite values. It is the shared reduction
scaffold the sample-reading primitives build on.
*/
func collectSamples(input types.Frame) ([]float64, bool) {
	var values []float64

	for index := range types.MaxSamples {
		value, present := input.Get(types.MustSampleSymbol(index))

		if !present {
			continue
		}

		if math.IsNaN(value) || math.IsInf(value, 0) {
			input.Err = fmt.Errorf("probability: sample/%d must be finite", index)

			return nil, false
		}

		values = append(values, value)
	}

	return values, true
}

/*
argmaxIndex returns the index of the largest value (ties to the lowest index).
*/
func argmaxIndex(values []float64) int {
	bestIndex := 0
	bestValue := values[0]

	for index := 1; index < len(values); index++ {
		if values[index] > bestValue {
			bestValue = values[index]
			bestIndex = index
		}
	}

	return bestIndex
}

/*
evidenceExponent returns a power-of-two exponent that bounds positive evidence
without changing any evidence-to-pseudocount ratio.
*/
func evidenceExponent(values []float64) int {
	maxEvidence := 0.0

	for _, value := range values {
		if value > maxEvidence {
			maxEvidence = value
		}
	}

	if maxEvidence <= 1 {
		return 0
	}

	_, exponent := math.Frexp(maxEvidence)

	return exponent
}

func finitePositive(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0
}

func clampUnit(value float64) float64 {
	if value < 0 {
		return 0
	}

	if value > 1 {
		return 1
	}

	return value
}
