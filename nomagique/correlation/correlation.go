package correlation

import (
	"github.com/theapemachine/symm/nomagique/arithmetic"
	"github.com/theapemachine/symm/nomagique/calculus"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/store"
)

/*
Correlation arranges a dependence estimate out of what the pairing and the
two paths' energies already yielded. It adds no operation of its own: the
covariance is addition over the paired products, the scale is the square root
of the two energies multiplied, and the estimate is one divided by the other.

Nothing normalizes the result into [-1, 1]. The cumulative estimator is not
bound to that range on a finite asynchronous sample, and a reading outside it
is a statement about how little the two paths co-sampled, which is worth
knowing rather than worth hiding.

Where the paths carry no energy the scale is zero, and Divide holds rather
than sending an infinity downstream, so the estimate reads as the covariance
it was given.

Everything passed in is a value some stage yielded, and everything handed
back is a Primitive, so this composes wherever a Primitive is accepted.
*/
func Correlation(products, leftEnergy, rightEnergy core.Primitive) core.Primitive {
	covariance := arithmetic.NewAdd(core.From(0.0)).Next(
		store.NewSpread(products),
	)

	scale := calculus.NewSqrt(core.From(0.0)).Next(
		arithmetic.NewMultiply(leftEnergy).Next(rightEnergy),
	)

	return arithmetic.NewDivide(covariance).Next(scale)
}
