package replay

import (
	"github.com/theapemachine/symm/execution"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func entryDeployFraction(
	costs ReplayCosts,
	act reasoning.Act,
	snapshots []types.Measurement,
) (float64, error) {
	return execution.EntryDeployFraction(execution.DeployFractionInput{
		PositionFraction: effectiveFraction(costs),
		ActFraction:      act.Fraction,
		Regime:           perspectives.ClassifyRegime(snapshots).Regime,
	})
}
