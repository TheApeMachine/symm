package replay

import (
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
entryDeployFraction scales the global position_fraction by the node's multiplier
and the structural regime. Choppy and bearish regimes deploy less capital so the
search co-evolves sizing with liquidity conditions.
*/
func entryDeployFraction(
	costs ReplayCosts,
	act reasoning.Act,
	snapshots []types.Measurement,
) (float64, error) {
	multiplier := 1.0

	if act.Fraction > 0 {
		multiplier = act.Fraction
	}

	fraction := effectiveFraction(costs) * multiplier

	if fraction <= 0 {
		return 0, nil
	}

	regime := perspectives.ClassifyRegime(snapshots).Regime
	scale, err := perspectives.RegimeSizeScale(regime)

	if err != nil {
		return 0, err
	}

	fraction *= scale

	if fraction < 0 {
		return 0, nil
	}

	return fraction, nil
}
