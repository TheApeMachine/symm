package execution

import (
	"fmt"

	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/types"
)

/*
DeployFractionInput is the shared sizing contract for live trader and optimizer replay.
Regime must be supplied by the caller: live uses the action's regime at emission;
replay classifies from the symbol window on that tick.
*/
type DeployFractionInput struct {
	PositionFraction float64
	ActFraction      float64
	Regime           types.Regime
}

/*
EntryDeployFraction scales global position_fraction by the playbook node multiplier
and structural regime — single source for trader and replay.
*/
func EntryDeployFraction(input DeployFractionInput) (float64, error) {
	multiplier := 1.0

	if input.ActFraction > 0 {
		multiplier = input.ActFraction
	}

	fraction := input.PositionFraction * multiplier

	if fraction <= 0 {
		return 0, nil
	}

	scale, err := perspectives.RegimeSizeScale(input.Regime)

	if err != nil {
		return 0, err
	}

	fraction *= scale

	if fraction < 0 {
		return 0, nil
	}

	if fraction > 1 {
		return 0, fmt.Errorf("execution: deploy fraction %.4f exceeds 1", fraction)
	}

	return fraction, nil
}

/*
EntryCapacity returns MaxConcurrentPositions. Deploy size is position_fraction;
concurrent count is no longer derived as floor(1/fraction).
*/
func EntryCapacity(fraction float64) int {
	if fraction <= 0 {
		return 0
	}

	return MaxConcurrentPositions()
}

/*
EntrySlotSpend is fee-inclusive quote spend for one entry (not coin quantity).
*/
func EntrySlotSpend(capital float64, fraction float64, feeRate float64, affordable float64) float64 {
	if capital <= 0 || fraction <= 0 {
		return 0
	}

	if affordable <= 0 {
		return 0
	}

	slot := fraction * capital / (1 + feeRate)
	affordableSpend := affordable / (1 + feeRate)

	if slot > affordableSpend {
		slot = affordableSpend
	}

	if slot <= 0 {
		return 0
	}

	return slot
}
