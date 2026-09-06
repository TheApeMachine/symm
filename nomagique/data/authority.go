package data

import (
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/equation"
	"github.com/theapemachine/symm/nomagique/store"
)

// NewAuthority preserves the source measurement's policy factors: .5 for an
// estimated reading with unknown SNR, .1 for defined non-positive SNR. They
// are retained compatibility policy, not calibrated probabilities.
func NewAuthority() core.Primitive {
	return equation.NewEvidenceAuthority(store.NewConstant(core.From(0.5)), store.NewConstant(core.From(0.1)))
}
