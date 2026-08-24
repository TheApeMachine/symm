package correlation

import (
	"math"

	"github.com/theapemachine/symm/nomagique/temporal"
	"github.com/theapemachine/symm/nomagique/types"
)

var (
	SymbolLagBars             = types.MustIntern("leadlag/lag_bars")
	SymbolLagCorrelation      = types.MustIntern("leadlag/lag_correlation")
	SymbolContempCorrelation  = types.MustIntern("leadlag/contemporaneous_correlation")
	SymbolLagFraction         = types.MustIntern("leadlag/lag_fraction")
	SymbolSignificance        = types.MustIntern("leadlag/significance")
	SymbolContempSignificance = types.MustIntern("leadlag/contemporaneous_significance")
	SymbolLagReady            = types.MustIntern("leadlag/lag_ready")
	SymbolLeadLagReady        = types.MustIntern("leadlag/ready")
	SymbolLeadLagSampleCount  = types.MustIntern("leadlag/sample_count")
	SymbolLeadLagSearchCount  = types.MustIntern("leadlag/search_count")
)

const minimumLagPathSamples = 3
const bonferroniTailFactor = 2

/*
CrossLag searches every shift that leaves enough retained returns to estimate
correlation. Hayashi evaluates each asynchronous pair and the actual search
count determines the Bonferroni threshold.

It is a bivariate primitive: the anchor path arrives in state and the follower
path in input, so the two operand series stay separate without any domain
naming. The result exposes the best lag, its correlation, the contemporaneous
correlation, the Bonferroni significance thresholds, and the readiness gate.
*/
func CrossLag(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	leftCount, _ := state.Get(types.SampleCount)
	rightCount, _ := input.Get(types.SampleCount)
	sampleCount := int(math.Min(leftCount, rightCount))
	output := input

	if sampleCount < minimumLagPathSamples {
		output.Put(SymbolLeadLagReady, 0)

		return state, output, nil
	}

	_, leftSpacing, err := temporal.Spacing(types.Frame{}, state)

	if err != nil {
		return state, types.Frame{}, err
	}

	_, rightSpacing, err := temporal.Spacing(types.Frame{}, input)

	if err != nil {
		return state, types.Frame{}, err
	}

	spacing := math.Min(
		leftSpacing.MustGet(temporal.SymbolSpacingNanos),
		rightSpacing.MustGet(temporal.SymbolSpacingNanos),
	)
	maximumLag := sampleCount - minimumLagPathSamples + 1
	lagInput := input
	lagInput.Put(SymbolLagSpacing, spacing)
	lagInput.Put(SymbolMaximumLag, float64(maximumLag))
	_, scan, err := Lag(state, lagInput)

	if err != nil {
		return state, types.Frame{}, err
	}

	bestLag := int(scan.MustGet(SymbolBestLag))
	bestCorrelation := scan.MustGet(SymbolBestLagCorrelation)
	bestMagnitude := math.Abs(bestCorrelation)
	contemporaneousCorrelation := scan.MustGet(SymbolContemporaneous)
	searchCount := int(scan.MustGet(SymbolSearchCount))

	if searchCount == 0 {
		output.Put(SymbolLeadLagReady, 0)

		return state, output, nil
	}

	returnCount := sampleCount - 1
	significance := math.Sqrt(
		bonferroniTailFactor * math.Log(float64(searchCount+1)) / float64(returnCount),
	)
	contemporaneousSignificance := math.Sqrt(
		bonferroniTailFactor * math.Log(2) / float64(returnCount),
	)
	lagReady := bestMagnitude > significance &&
		bestMagnitude > math.Abs(contemporaneousCorrelation)
	lagFraction := 0.0

	if lagReady {
		lagFraction = math.Abs(float64(bestLag)) / float64(maximumLag)
	}

	output.Put(SymbolLagBars, float64(bestLag))
	output.Put(SymbolLagCorrelation, bestCorrelation)
	output.Put(SymbolContempCorrelation, contemporaneousCorrelation)
	output.Put(SymbolLagFraction, lagFraction)
	output.Put(SymbolSignificance, significance)
	output.Put(SymbolContempSignificance, contemporaneousSignificance)
	output.Put(SymbolLagReady, truth(lagReady))
	output.Put(SymbolLeadLagReady, 1)
	output.Put(SymbolLeadLagSampleCount, float64(sampleCount))
	output.Put(SymbolLeadLagSearchCount, float64(searchCount))

	return state, output, nil
}
