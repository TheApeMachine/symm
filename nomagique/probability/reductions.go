package probability

import (
	"math"

	"github.com/theapemachine/symm/nomagique/types"
)

/*
Geomean is the geometric mean of a collection of non-negative values: the
central tendency appropriate to quantities that combine multiplicatively,
where one near-zero member should drag the aggregate down rather than being
averaged away.

It is a pure Reduction-tier fold — no state, no configuration. An empty
collection has no central tendency and reduces to zero; a negative member
makes the geometric mean undefined and likewise reduces to zero.

The product is accumulated in log space so a long collection cannot
underflow to zero before the root is taken.
*/
func Geomean(values []types.Scalar) types.Scalar {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0

	for _, value := range values {
		if value < 0 || math.IsNaN(float64(value)) {
			return 0
		}

		// A single zero member zeroes the whole product; short-circuit rather
		// than taking log(0).
		if value == 0 {
			return 0
		}

		sum += math.Log(float64(value))
	}

	return types.Scalar(math.Exp(sum / float64(len(values))))
}

/*
ShannonAmbiguity is the normalized Shannon entropy of a collection of
non-negative weights: zero when all the mass sits on one member, one when it
is spread uniformly across every member.

The weights are normalized into a distribution first, so the caller may pass
unnormalized affinities or scores. Normalizing by log(k) — the entropy of the
uniform distribution over exactly the members present — makes the reading
comparable across collections of different sizes.
*/
func ShannonAmbiguity(values []types.Scalar) types.Scalar {
	if len(values) < 2 {
		return 0
	}

	total := 0.0

	for _, value := range values {
		if value < 0 || math.IsNaN(float64(value)) {
			return 0
		}

		total += float64(value)
	}

	if total <= 0 {
		return 0
	}

	entropy := 0.0

	for _, value := range values {
		probability := float64(value) / total

		if probability > 0 {
			entropy -= probability * math.Log(probability)
		}
	}

	return types.Scalar(entropy / math.Log(float64(len(values))))
}

/*
EvidenceShare returns one member's share of the total mass: how much of the
collective evidence that member alone accounts for.

It reports zero for an index outside the collection or a collection carrying
no mass, so an absent member never appears to hold a share.
*/
func EvidenceShare(values []types.Scalar, index int) types.Scalar {
	if index < 0 || index >= len(values) {
		return 0
	}

	total := 0.0

	for _, value := range values {
		if value < 0 || math.IsNaN(float64(value)) {
			return 0
		}

		total += float64(value)
	}

	if total <= 0 {
		return 0
	}

	return types.Scalar(float64(values[index]) / total)
}

/*
Argmax returns the index of the largest member and its value, reporting false
for an empty collection. Ties resolve to the first such member so the reading
is deterministic.
*/
func Argmax(values []types.Scalar) (int, types.Scalar, bool) {
	if len(values) == 0 {
		return 0, 0, false
	}

	winner := 0
	best := values[0]

	for index := 1; index < len(values); index++ {
		if values[index] > best {
			best = values[index]
			winner = index
		}
	}

	return winner, best, true
}

// Geomean, ShannonAmbiguity satisfy the pure Reduction contract.
var (
	_ types.Reduction = Geomean
	_ types.Reduction = ShannonAmbiguity
)
