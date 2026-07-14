package logic

import (
	"math"

	"github.com/theapemachine/symm/types"
)

func (composer *CategoryComposer) liquidityState(
	symbol string,
	measurements []*types.Measurement,
	graph *types.Graph,
) (types.Category, bool) {
	scarcity, hasScarcity := latestMeasurement(
		symbol, measurements,
		types.SubjectPeerLiquidity, types.MetricScarcityScore,
	)
	depth, hasDepth := latestMeasurement(
		symbol, measurements,
		types.SubjectPeerLiquidity, types.MetricDepthScore,
	)

	if !hasScarcity || !hasDepth {
		return types.Category{}, false
	}

	scarcityWeight := weightedValue(*scarcity)
	depthWeight := weightedValue(*depth)
	totalWeight := scarcityWeight + depthWeight

	if totalWeight <= 0 {
		return types.Category{}, false
	}

	categoryType := types.CategoryRobustLiquidity

	if scarcityWeight >= depthWeight {
		categoryType = types.CategoryExtremeScarcity

		if scarcity.Raw > depth.Raw {
			categoryType = types.CategoryLiquidityVacuum
		}
	}

	anchor := scarcity

	if depthWeight > scarcityWeight {
		anchor = depth
	}

	anchorKey := types.MeasurementKey(anchor)
	supporting, opposing := graphEvidence(graph, anchorKey)
	missing := missingSubjects(symbol, measurements, []types.SubjectType{
		types.SubjectPumpVolumeLift,
		types.SubjectLevel3Touch,
	})

	strength := math.Max(scarcityWeight, depthWeight) / totalWeight
	confidence := evidenceConfidence(
		anchor.Maturity,
		len(supporting),
		len(opposing),
		len(missing),
	)

	return types.Category{
		Symbol:     symbol,
		Type:       categoryType,
		Strength:   strength,
		Confidence: confidence,
		Surprisal:  1 - confidence,
		Maturity:   anchor.Maturity,
		Supporting: supporting,
		Opposing:   opposing,
		Missing:    missing,
	}, true
}
