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
	SymbolLagSearchResolution = types.MustIntern("leadlag/lag_search_resolution_nanos")
	SymbolLagSearchSpan       = types.MustIntern("leadlag/lag_search_span_nanos")
	SymbolBestLagNanos        = types.MustIntern("leadlag/best_lag_nanos")
	SymbolLagPeakProminence   = types.MustIntern("leadlag/lag_peak_prominence")
	SymbolLagPeakCurvature    = types.MustIntern("leadlag/lag_peak_curvature")
	SymbolMeasuredReturns     = types.MustIntern("leadlag/measured_return_count")
	SymbolReferenceReturns    = types.MustIntern("leadlag/reference_return_count")
	SymbolOverlapPairs        = types.MustIntern("leadlag/overlap_pair_count")
	SymbolEffectiveSupport    = types.MustIntern("leadlag/effective_sample_count")
)

const minimumLagPathSamples = 3
const bonferroniTailFactor = 2

/*
CrossLag returns the primitive that searches every shift between the leftPrefix
and rightPrefix series that leaves enough retained returns to estimate
correlation. Hayashi evaluates each asynchronous pair and the actual search
count determines the Bonferroni threshold. The result exposes the best lag, its
correlation, the contemporaneous correlation, the Bonferroni significance
thresholds, and the readiness gate.
*/
func CrossLag(leftPrefix string, rightPrefix string) types.Primitive {
	leftSeries := temporal.NewSeries(leftPrefix)
	rightSeries := temporal.NewSeries(rightPrefix)

	return func(input types.Frame) types.Frame {
		leftCount := leftSeries.Count(input)
		rightCount := rightSeries.Count(input)
		sampleCount := int(math.Min(float64(leftCount), float64(rightCount)))

		if sampleCount < minimumLagPathSamples {
			input.Put(SymbolLeadLagReady, 0)

			return input
		}

		leftSpacing := leftSeries.Spacing(input)

		if leftSpacing.Err != nil {
			input.Err = leftSpacing.Err

			return input
		}

		rightSpacing := rightSeries.Spacing(input)

		if rightSpacing.Err != nil {
			input.Err = rightSpacing.Err

			return input
		}

		spacing := math.Min(
			leftSpacing.MustGet(temporal.SymbolSpacingNanos),
			rightSpacing.MustGet(temporal.SymbolSpacingNanos),
		)
		maximumLag := sampleCount - minimumLagPathSamples + 1
		lagInput := input
		lagInput.Put(SymbolLagSpacing, spacing)
		lagInput.Put(SymbolMaximumLag, float64(maximumLag))
		scan := Lag(leftPrefix, rightPrefix)(lagInput)

		if scan.Err != nil {
			input.Err = scan.Err

			return input
		}

		bestLag := int(scan.MustGet(SymbolBestLag))
		bestCorrelation := scan.MustGet(SymbolBestLagCorrelation)
		bestMagnitude := math.Abs(bestCorrelation)
		contemporaneousCorrelation := scan.MustGet(SymbolContemporaneous)
		searchCount := int(scan.MustGet(SymbolSearchCount))

		if searchCount == 0 {
			input.Put(SymbolLeadLagReady, 0)

			return input
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

		input.Put(SymbolLagBars, float64(bestLag))
		input.Put(SymbolLagCorrelation, bestCorrelation)
		input.Put(SymbolContempCorrelation, contemporaneousCorrelation)
		input.Put(SymbolLagFraction, lagFraction)
		input.Put(SymbolSignificance, significance)
		input.Put(SymbolContempSignificance, contemporaneousSignificance)
		input.Put(SymbolLagReady, truth(lagReady))
		input.Put(SymbolLeadLagReady, 1)
		input.Put(SymbolLeadLagSampleCount, float64(sampleCount))
		input.Put(SymbolLeadLagSearchCount, float64(searchCount))
		input.Put(SymbolLagSearchResolution, spacing)
		input.Put(SymbolLagSearchSpan, spacing*float64(maximumLag))
		input.Put(SymbolBestLagNanos, float64(bestLag)*spacing)
		input.Put(SymbolOverlapPairs, scan.MustGet(SymbolBestLagSupport))
		input.Put(SymbolEffectiveSupport, scan.MustGet(SymbolBestLagSupport))
		input.Put(SymbolMeasuredReturns, float64(rightCount-1))
		input.Put(SymbolReferenceReturns, float64(leftCount-1))

		if neighborLow, found := scan.Get(SymbolNeighborLow); found {
			if neighborHigh, foundHigh := scan.Get(SymbolNeighborHigh); foundHigh {
				magnitude := math.Abs(bestCorrelation)
				prominence := magnitude - (math.Abs(neighborLow)+math.Abs(neighborHigh))/2
				resolutionSeconds := spacing / 1e9
				curvature := (2*magnitude - math.Abs(neighborLow) - math.Abs(neighborHigh)) /
					(resolutionSeconds * resolutionSeconds)
				input.Put(SymbolLagPeakProminence, prominence)
				input.Put(SymbolLagPeakCurvature, curvature)
			}
		}

		return input
	}
}
