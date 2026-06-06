package market

import (
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/qpool"
	"github.com/theapemachine/symm/market/perspectives"
	"github.com/theapemachine/symm/market/perspectives/reasoning"
	"github.com/theapemachine/symm/market/perspectives/types"
	"github.com/theapemachine/symm/numeric"
)

const (
	storyUIInterval = 500 * time.Millisecond
	marketSymbol    = "market"
)

var predictionDefaultBandEdges = []float64{0.75, 1.0, 1.5}

/*
publishPredictionGauge ships the cross-section entry thesis multiple as the Pred
gauge. Only symbols whose playbook currently authorizes entry contribute, and
zero ratios are excluded from the average.
*/
func (story *Story) publishPredictionGauge() {
	if story.ui == nil || story.predictionCalibrator == nil {
		return
	}

	ratios := story.collectEntryMultiples()

	if len(ratios) == 0 {
		return
	}

	sum := 0.0

	for _, ratio := range ratios {
		sum += ratio
	}

	observation := sum / float64(len(ratios))
	measurement := types.Measurement{
		Source:     types.SourcePrediction,
		Symbol:     marketSymbol,
		Category:   predictionCategory(observation),
		Strength:   observation,
		Confidence: math.Min(1, observation/2),
	}

	telemetry, standout := numeric.ObserveGaugeTelemetry(
		story.predictionCalibrator.Calibrator,
		story.predictionCalibrator.Classifier,
		observation,
		math.Min(1, observation/3),
	)

	if err := types.AssignCategorySNR(
		&measurement, story.predictionFloor, standout,
	); err != nil {
		return
	}

	story.ui.Send(&qpool.QValue[any]{
		Value: numeric.GaugePayload(
			types.SourcePrediction.String(),
			marketSymbol,
			measurement.Category,
			measurement,
			telemetry,
		),
	})
}

func (story *Story) collectEntryMultiples() []float64 {
	if len(story.thoughts) == 0 {
		return nil
	}

	symbols := story.symbolsSeen()
	ratios := make([]float64, 0, len(symbols))

	for _, symbol := range symbols {
		snapshots := make([]types.Measurement, 0, story.ringWindow.Len())

		story.ringWindow.Do(func(value any) {
			if measurement, ok := value.(types.Measurement); ok {
				if measurement.Symbol == symbol {
					snapshots = append(snapshots, measurement)
				}
			}
		})

		regime := perspectives.ClassifyRegime(snapshots)
		context := story.windowReason.Reset(
			snapshots, regime.Regime, story.positionState(types.Measurement{Symbol: symbol}),
		)

		act, found := reasoning.EvaluateStatefulTraced(
			story.thoughts, context, story.reasonState(symbol), nil,
		)

		if !found || !reasoning.IsEntryAction(act.Type) {
			continue
		}

		thesis := thesisScoreRMS(snapshots)
		required := requiredEntryScore(snapshots)

		if thesis <= 0 || required <= 0 {
			continue
		}

		ratio := thesis / required

		if ratio <= 0 || math.IsNaN(ratio) || math.IsInf(ratio, 0) {
			continue
		}

		ratios = append(ratios, ratio)
	}

	return ratios
}

func (story *Story) symbolsSeen() []string {
	seen := make(map[string]struct{})

	story.ringWindow.Do(func(value any) {
		if measurement, ok := value.(types.Measurement); ok {
			if measurement.Symbol == "" {
				return
			}

			seen[measurement.Symbol] = struct{}{}
		}
	})

	symbols := make([]string, 0, len(seen))

	for symbol := range seen {
		symbols = append(symbols, symbol)
	}

	return symbols
}

func thesisScoreRMS(snapshots []types.Measurement) float64 {
	sumSquares := 0.0
	count := 0

	for _, measurement := range snapshots {
		if measurement.SNR <= 0 || math.IsNaN(measurement.SNR) || math.IsInf(measurement.SNR, 0) {
			continue
		}

		sumSquares += measurement.SNR * measurement.SNR
		count++
	}

	if count == 0 {
		return 0
	}

	return math.Sqrt(sumSquares / float64(count))
}

func requiredEntryScore(snapshots []types.Measurement) float64 {
	multiple := viper.GetFloat64("trading.entry_edge_multiple")

	if multiple <= 0 {
		multiple = 2
	}

	return multiple * roundTripFrictionPct(snapshots) * 100
}

func roundTripFrictionPct(snapshots []types.Measurement) float64 {
	spreadBPS := 0.0

	for _, measurement := range snapshots {
		if measurement.SpreadBPS > spreadBPS {
			spreadBPS = measurement.SpreadBPS
		}
	}

	takerFeePct := viper.GetFloat64("trading.paper.taker_fee_pct")

	if takerFeePct <= 0 {
		takerFeePct = viper.GetFloat64("trading.taker_fee_pct")
	}

	return 2*takerFeePct + spreadBPS/100
}

func predictionCategory(ratio float64) types.CategoryType {
	switch {
	case ratio >= 1.5:
		return types.CategoryOrganicTrend
	case ratio >= 1:
		return types.CategorySynchronizedDrift
	case ratio >= 0.75:
		return types.CategoryStochasticBalance
	default:
		return types.CategoryStochasticNoise
	}
}
