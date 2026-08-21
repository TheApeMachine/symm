package statistic

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

var SymbolSeparation = nomagique.MustIntern("separation")

/*
Separation reports the normalized margin between two non-negative competing
hypotheses. No evidence separates nothing; one supported hypothesis separates
completely; equal support has zero separation.
*/
func Separation(
	state nomagique.Frame,
	input nomagique.Frame,
) (nomagique.Frame, nomagique.Frame, error) {
	alpha, hasAlpha := input.Get(nmtypes.AlphaQuantity)
	beta, hasBeta := input.Get(nmtypes.BetaQuantity)

	if !hasAlpha || !hasBeta || alpha < 0 || beta < 0 {
		return state, nomagique.Frame{}, fmt.Errorf(
			"statistic: separation requires non-negative alpha and beta quantities",
		)
	}
	winner := math.Max(alpha, beta)
	separation := 0.0

	if winner > 0 {
		separation = (winner - math.Min(alpha, beta)) / winner
	}

	output := input
	output.Put(SymbolSeparation, separation)

	return state, output, nil
}
