package probability

import "math"

/*
Ranked is one token-probability pair for sampling or beam expansion.
*/
type Ranked struct {
	Token       string
	Probability float64
}

/*
TemperatureShape applies temperature scaling to a probability distribution.
Temperature 0 produces a greedy (argmax-uniform) distribution.
Positive temperature reshapes via power scaling and renormalization.
*/
func TemperatureShape(distribution []Ranked, temperature float64) []Ranked {
	if len(distribution) == 0 {
		return nil
	}

	clone := make([]Ranked, len(distribution))
	copy(clone, distribution)

	if temperature <= 0 {
		return greedyShape(clone)
	}

	total := 0.0

	for index := range clone {
		clone[index].Probability = math.Pow(
			clone[index].Probability,
			1/temperature,
		)
		total += clone[index].Probability
	}

	if total == 0 {
		return nil
	}

	for index := range clone {
		clone[index].Probability /= total
	}

	return clone
}

func greedyShape(distribution []Ranked) []Ranked {
	maxProb := distribution[0].Probability

	for _, entry := range distribution[1:] {
		if entry.Probability > maxProb {
			maxProb = entry.Probability
		}
	}

	bestCount := 0

	for _, entry := range distribution {
		if entry.Probability == maxProb {
			bestCount++
		}
	}

	out := make([]Ranked, len(distribution))

	for index := range distribution {
		if distribution[index].Probability == maxProb {
			out[index] = Ranked{
				Token:       distribution[index].Token,
				Probability: 1 / float64(bestCount),
			}

			continue
		}

		out[index] = Ranked{
			Token:       distribution[index].Token,
			Probability: 0,
		}
	}

	return out
}

/*
NormalizeMap rescales a string-keyed probability map so values sum to 1.
*/
func NormalizeMap(values map[string]float64) {
	total := 0.0

	for _, probability := range values {
		total += probability
	}

	if total == 0 {
		return
	}

	for token := range values {
		values[token] /= total
	}
}

/*
AdditiveSmoothing returns P(token) under additive (Laplace) smoothing:
(count + smoothing) / (total + smoothing * vocabSize).
*/
func AdditiveSmoothing(count float64, total float64, vocabSize int, smoothing float64) float64 {
	if vocabSize < 0 || smoothing < 0 {
		return math.NaN()
	}

	denom := total + smoothing*float64(vocabSize)

	if denom == 0 {
		return 0
	}

	return (count + smoothing) / denom
}

/*
RepetitionPenalty scales down probabilities of recently seen tokens and renormalizes.
*/
func RepetitionPenalty(distribution []Ranked, recentTokens []string, penaltyWeight float64) []Ranked {
	if len(distribution) == 0 || len(recentTokens) == 0 {
		return distribution
	}

	const penaltyMin = 1e-9

	penaltyWeight = math.Max(penaltyMin, math.Min(1, penaltyWeight))

	recent := make(map[string]struct{}, len(recentTokens))

	for _, token := range recentTokens {
		recent[token] = struct{}{}
	}

	adjusted := make([]Ranked, 0, len(distribution))
	total := 0.0

	for _, entry := range distribution {
		penalty := 1.0

		if _, exists := recent[entry.Token]; exists {
			penalty = penaltyWeight
		}

		prob := entry.Probability * penalty
		adjusted = append(adjusted, Ranked{
			Token:       entry.Token,
			Probability: prob,
		})
		total += prob
	}

	if total == 0 {
		return distribution
	}

	for index := range adjusted {
		adjusted[index].Probability /= total
	}

	return adjusted
}

/*
Surprisal returns the Shannon surprisal in bits: -log2(p).
Falls back to floor when p <= 0.
*/
func Surprisal(probability float64, floor float64) float64 {
	if probability <= 0 {
		probability = floor
	}

	return -math.Log2(probability)
}
