package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var SymbolSeparation = nmtypes.MustIntern("separation")

/*
Separation reports the normalized margin between two non-negative competing
hypotheses. No evidence separates nothing; one supported hypothesis separates
completely; equal support has zero separation.
*/
func Separation(input *types.Frame) {
	alpha, hasAlpha := input.Get(nmtypes.AlphaQuantity)
	beta, hasBeta := input.Get(nmtypes.BetaQuantity)

	if !hasAlpha || !hasBeta || alpha < 0 || beta < 0 {
		input.Err = fmt.Errorf(
			"statistic: separation requires non-negative alpha and beta quantities",
		)

		return
	}

	winner := math.Max(alpha, beta)
	separation := 0.0

	if winner > 0 {
		separation = (winner - math.Min(alpha, beta)) / winner
	}

	input.Put(SymbolSeparation, separation)
}
