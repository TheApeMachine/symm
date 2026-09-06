package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
)

/*
Fisher arranges the significance of a correlation: the tail mass a
correlation at least this strong would have under no relationship at all.

A correlation cannot be tested where it lives, because its sampling
dispersion collapses toward the boundaries. The Fisher transform maps it onto
the whole real line where the dispersion is stable, the transformed estimate
is scaled by the support behind it, and the standard normal tail is read off
the result.

It is composition rather than a Primitive, because every operation it needs
already exists: an inverse hyperbolic tangent, a subtraction, a square root,
a multiplication, a magnitude, a division, and the complementary error
function.

Support carries the estimate. Below four paired observations the square root
runs out of domain, the scaled statistic is zero, and the tail mass is one:
not significant, which is the honest reading of an estimate nothing supports.

No cutoff is embedded. Fisher reports the tail mass and the consumer decides
what to do about it.
*/
func Fisher(correlation, support core.Primitive) core.Primitive {
	degrees := calculus.NewSqrt(core.From(0.0)).Next(
		arithmetic.NewSubtract(support).Next(core.From(3.0)),
	)

	statistic := arithmetic.NewMultiply(
		calculus.NewAtanh(core.From(0.0)).Next(correlation),
	).Next(degrees)

	return calculus.NewErfc(core.From(0.0)).Next(
		arithmetic.NewDivide(
			calculus.NewAbsolute(core.From(0.0)).Next(statistic),
		).Next(core.From(math.Sqrt2)),
	)
}

/*
Bonferroni arranges the correction for having taken the best of many looks. A
tail mass earned by searching M candidates is worth M times less, bounded at
one, because a probability cannot exceed certainty.
*/
func Bonferroni(significance, candidates core.Primitive) core.Primitive {
	return calculus.NewMinimum(core.From(1.0)).Next(
		arithmetic.NewMultiply(significance).Next(candidates),
	)
}
