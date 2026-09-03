package equation

import (
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Polarize decomposes a signed scalar into reciprocal non-negative components
(Alpha and Beta) and normalizes them against an adaptive scale.
Tier 4 Equation: zero Wire blocks, zero Frame allocations, zero MustIntern symbols.
*/
type Polarize struct {
	alpha           types.Scalar
	beta            types.Scalar
	alphaNormalized types.Scalar
	betaNormalized  types.Scalar
}

func (p *Polarize) Step(x types.Scalar) types.Scalar {
	return p.StepScaled(x, 1.0)
}

func (p *Polarize) StepScaled(x, scale types.Scalar) types.Scalar {
	if x > 0 {
		p.alpha = x
		p.beta = 0
	} else {
		p.alpha = 0
		p.beta = -x
	}

	if scale > 0 {
		p.alphaNormalized = p.alpha / (p.alpha + scale)
		p.betaNormalized = p.beta / (p.beta + scale)
	} else {
		p.alphaNormalized = 0
		p.betaNormalized = 0
	}

	return p.alphaNormalized - p.betaNormalized
}

func (p *Polarize) Alpha() types.Scalar           { return p.alpha }
func (p *Polarize) Beta() types.Scalar            { return p.beta }
func (p *Polarize) AlphaNormalized() types.Scalar { return p.alphaNormalized }
func (p *Polarize) BetaNormalized() types.Scalar  { return p.betaNormalized }
