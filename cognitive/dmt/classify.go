package dmt

import (
	"math"
	"sort"
	"strings"
)

/*
Classify evaluates a sensory sequence against learned attractor basins.
*/
func (tree *Tree) Classify(
	sequence []byte,
	scratch *ClassificationScratch,
) ClassificationResult {
	if tree == nil || len(sequence) == 0 {
		return ClassificationResult{}
	}

	tree.mu.RLock()
	defer tree.mu.RUnlock()

	sequenceKey := string(sequence)
	counts := make(map[string]uint64)
	total := uint64(0)

	for className, classBasins := range tree.basins {
		state := classBasins[sequenceKey]
		if state.Count == 0 {
			continue
		}

		counts[className] += state.Count
		total += state.Count
	}

	if total == 0 {
		return ClassificationResult{}
	}

	scores := make([]ClassScore, 0, len(counts))
	for className, count := range counts {
		scores = append(scores, ClassScore{
			ClassName: []byte(className),
			Value:     probability(count, total),
		})
	}

	sort.SliceStable(scores, func(left, right int) bool {
		if scores[left].Value == scores[right].Value {
			return string(scores[left].ClassName) < string(scores[right].ClassName)
		}

		return scores[left].Value > scores[right].Value
	})

	return ClassificationResult{
		Scores:  scores,
		Winner:  scores[0].ClassName,
		Highest: scores[0].Value,
	}
}

/*
PredictNextSensoryTokens returns immediate children below a sequence prefix.
*/
func (tree *Tree) PredictNextSensoryTokens(
	sequencePrefix []byte,
	targetBuffer []LookaheadPrediction,
) []LookaheadPrediction {
	if tree == nil {
		return targetBuffer[:0]
	}

	tree.mu.RLock()
	defer tree.mu.RUnlock()

	targetBuffer = targetBuffer[:0]
	byToken := make(map[string]float64)
	prefix := string(sequencePrefix)

	for sequence, state := range tree.sensory {
		token, ok := immediateChild(prefix, sequence)
		if !ok {
			continue
		}

		if state.Probability <= byToken[token] {
			continue
		}

		byToken[token] = state.Probability
	}

	tokens := make([]string, 0, len(byToken))
	for token := range byToken {
		tokens = append(tokens, token)
	}

	sort.Strings(tokens)

	for _, token := range tokens {
		targetBuffer = append(targetBuffer, LookaheadPrediction{
			Token:       []byte(token),
			Probability: byToken[token],
		})

		if cap(targetBuffer) > 0 && len(targetBuffer) == cap(targetBuffer) {
			break
		}
	}

	return targetBuffer
}

/*
ExecuteBeamSearch expands sensory lookahead paths from a starting prefix.
*/
func (tree *Tree) ExecuteBeamSearch(
	prefix []byte,
	width int,
	maxHops int,
	scratch *BeamSearchScratch,
) []BeamPath {
	if tree == nil || width <= 0 || maxHops <= 0 {
		return nil
	}

	if scratch == nil {
		scratch = &BeamSearchScratch{}
	}

	current := scratch.CurrentBeams[:0]
	current = append(current, BeamPath{
		Sequence: append([]byte(nil), prefix...),
		Score:    0,
	})

	lookup := scratch.LookupBuffer[:0]

	for depth := 0; depth < maxHops; depth++ {
		next := scratch.NextBeams[:0]

		for _, beam := range current {
			lookup = tree.PredictNextSensoryTokens(beam.Sequence, lookup[:0])
			for _, prediction := range lookup {
				score := logScore(prediction.Probability)
				if math.IsInf(score, -1) {
					continue
				}

				next = append(next, BeamPath{
					Sequence: appendToken(nil, beam.Sequence, prediction.Token),
					Score:    beam.Score + score,
				})
			}
		}

		if len(next) == 0 {
			break
		}

		sortBeams(next)
		if len(next) > width {
			next = next[:width]
		}

		current, next = next, current[:0]
		scratch.NextBeams = next
	}

	sortBeams(current)
	scratch.CurrentBeams = current
	scratch.LookupBuffer = lookup[:0]

	out := make([]BeamPath, 0, len(current))
	for _, beam := range current {
		if len(beam.Sequence) == 0 {
			continue
		}

		out = append(out, BeamPath{
			Sequence: append([]byte(nil), beam.Sequence...),
			Score:    beam.Score,
		})
	}

	return out
}

func immediateChild(prefix string, sequence string) (string, bool) {
	if prefix == "" {
		if sequence == "" {
			return "", false
		}

		token, _, _ := strings.Cut(sequence, "_")
		return token, token != ""
	}

	if sequence == prefix {
		return "", false
	}

	remainder, ok := strings.CutPrefix(sequence, prefix+"_")
	if !ok || remainder == "" {
		return "", false
	}

	token, _, _ := strings.Cut(remainder, "_")

	return token, token != ""
}
