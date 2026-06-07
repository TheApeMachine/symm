package reasoning

import (
	"strconv"
	"strings"

	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
ResolveConviction returns the SNR and confidence of the signal evidence that
actually fired a node: among the signal leaves of the fired node's When, the
one with the strongest snr×confidence right now. Before this, an action was
stamped from whatever measurement row happened to trigger the walk — a
quick_pump entry latched on pumpdump evidence could carry a depthflow tick's
near-zero SNR into entry ranking and preemption, comparing incomparable scores.

Falls back to the ambient measurement when the node has no signal leaves (pure
price/position nodes) or the key cannot be resolved.
*/
func ResolveConviction(
	thoughts []Thought,
	firedKey string,
	ctx ReasonContext,
	fallback types.Measurement,
) (float64, float64) {
	snr, confidence := fallback.SNR, fallback.Confidence
	node, ok := thoughtByKey(thoughts, firedKey)

	if !ok {
		return snr, confidence
	}

	bestScore := -1.0

	for _, leaf := range FlattenLeaves(node.When) {
		predicate := leaf.Predicate

		if predicate.Subject != SubjectSignal || predicate.Category == types.CategoryTypeNone || leaf.Negated {
			continue
		}

		leafSNR, okSNR := ctx.Signal(predicate.Category, UnitSNR, Lookback{})
		leafConfidence, okConf := ctx.Signal(predicate.Category, UnitConfidence, Lookback{})

		if !okSNR || !okConf {
			continue
		}

		if score := leafSNR * leafConfidence; score > bestScore {
			bestScore, snr, confidence = score, leafSNR, leafConfidence
		}
	}

	return snr, confidence
}

// thoughtByKey walks the dotted index path produced by the evaluator ("0.2.1").
func thoughtByKey(thoughts []Thought, key string) (Thought, bool) {
	if key == "" {
		return Thought{}, false
	}

	nodes := thoughts
	var node Thought

	for _, part := range strings.Split(key, ".") {
		index, err := strconv.Atoi(part)

		if err != nil || index < 0 || index >= len(nodes) {
			return Thought{}, false
		}

		node = nodes[index]
		nodes = node.Then
	}

	return node, true
}
