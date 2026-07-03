package correlation

import (
	"context"
	"iter"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
Signal: The "Herd Behavior" Perspective

What it measures exactly (in isolation)

The Correlation signal measures synchronized return correlation across the subscribed universe using rolling per-symbol log returns from the market cross-section.
It determines if the market is moving as a single, indistinguishable block or if individual assets are exhibiting unique behavior.

*   Synchronized Log-Returns: It aligns price windows through CrossSection peer samples.
*   Peer Cache: It compares each symbol's returns to the median peer-return path, excluding the symbol being measured.
*   Relative Energy: It separates quiet low-correlation noise from high-energy decoupled alpha or divergent stress.

Semantically, what story does it tell?

*   The "Rising Tide" Story: It asks: "Is this asset special, or is it just being dragged along by the herd?". High correlation indicates that macro-systemic forces are dominant.
*   The "De-coupling" Story: It identifies "alpha" opportunities by spotting when an asset stops following its peers, suggesting a local catalyst is at play.
*   The "Liquidation" Story: Negative high-energy correlation can mark divergent stress or broad liquidation mechanics.

# Probability Visualization Categories

| Category         | Correlation Level | Variance | Market "Feel"                       |
|:-----------------|:------------------|:---------|:------------------------------------|
| Systemic Herd    | Positive          | High     | Global Beta / Momentum Drift        |
| Decoupled Alpha  | Low               | High     | Unique Driver / Leading Move        |
| Stochastic Noise | Low               | Low      | Quiet / Indecisive                  |
| Divergent Stress | Negative          | High     | Contrarian Move / Relative Weakness |
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	tree       *dmt.Tree
	classifier *probability.Classifier
}

func NewSignal(ctx context.Context, tree *dmt.Tree) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		classifier: probability.NewClassifier(datura.Acquire(
			"correlation", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"herdScore",
				"alphaScore",
				"noiseScore",
				"stressScore",
			},
			"categoryIndexes": []float64{
				float64(logic.CategoryIndex(logic.CategorySystemicHerd)),
				float64(logic.CategoryIndex(logic.CategoryDecoupledAlpha)),
				float64(logic.CategoryIndex(logic.CategoryStochasticNoise)),
				float64(logic.CategoryIndex(logic.CategoryDivergentStress)),
			},
		})),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil || crossSection == nil {
			return
		}

		if channel := datura.Peek[string](datapoint, "channel"); channel != "" && channel != "ticker" {
			return
		}

		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")

			if symbol == "" {
				return
			}

			measurement := signal.MeasureRow(datapoint, rowIndex, crossSection)

			if measurement == nil {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) MeasureRow(
	datapoint *datura.Artifact,
	rowIndex int,
	crossSection *market.CrossSection,
) *datura.Artifact {
	symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")

	if symbol == "" {
		return nil
	}

	correlation, energy, peerCorrelations, peerEnergyMedian, ok := crossSection.PeerCache.SymbolStats(
		crossSection,
		symbol,
		crossSection.MaxReturnWindow(),
	)

	if !ok {
		return nil
	}

	return signal.WriteMeasurement(
		datapoint,
		symbol,
		correlation,
		energy,
		peerCorrelations,
		peerEnergyMedian,
	)
}

func (signal *Signal) WriteMeasurement(
	datapoint *datura.Artifact,
	symbol string,
	correlation float64,
	energy float64,
	peerCorrelations []float64,
	peerEnergyMedian float64,
) *datura.Artifact {
	if len(peerCorrelations) == 0 {
		return nil
	}

	herdScore := signal.HerdScore(correlation, energy, peerCorrelations, peerEnergyMedian)
	alphaScore := signal.AlphaScore(correlation, energy, peerEnergyMedian)
	noiseScore := signal.NoiseScore(correlation, energy, peerEnergyMedian)
	stressScore := signal.StressScore(correlation, energy, peerEnergyMedian)
	peakScore := max(max(herdScore, alphaScore), max(noiseScore, stressScore))

	if peakScore <= 0 {
		return nil
	}

	peerCorrelationMedian, _ := statistic.MedianOf(peerCorrelations)
	measurement := datura.Acquire("correlation", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceCorrelation)))
	measurement.SetTimestamp(datapoint.Timestamp())
	measurement.MergeOutputs(map[string]any{
		"correlation":           correlation,
		"energy":                energy,
		"peerEnergyMedian":      peerEnergyMedian,
		"peerCorrelationMedian": peerCorrelationMedian,
		"peakScore":             peakScore,
		"herdScore":             herdScore,
		"alphaScore":            alphaScore,
		"noiseScore":            noiseScore,
		"stressScore":           stressScore,
		"strength":              peakScore,
	})
	measurement.Poke("output", "root")
	measurement.Poke([]string{
		"herdScore",
		"alphaScore",
		"noiseScore",
		"stressScore",
		"strength",
	}, "inputs")

	if err := signal.classifier.Apply(measurement); err != nil {
		measurement.WithError(errnie.Error(err))
		return measurement
	}

	if datura.Peek[float64](measurement, "output", "confidence") <= 0 {
		measurement.Release()
		return nil
	}

	return measurement
}

func (signal *Signal) HerdScore(
	correlation float64,
	energy float64,
	peerCorrelations []float64,
	peerEnergyMedian float64,
) float64 {
	alignment := math.Max(0, correlation)
	positive := signal.Positive(peerCorrelations)
	relativeCorrelation := 0.0

	if len(positive) == 0 {
		if alignment > 0 {
			relativeCorrelation = 1
		}
	} else if median, ok := statistic.MedianOf(positive); ok && median > 0 {
		relativeCorrelation = alignment / median
	}

	energyLift := signal.EnergyLift(energy, peerEnergyMedian)

	return signal.Bounded(alignment * relativeCorrelation * energyLift)
}

func (signal *Signal) AlphaScore(
	correlation float64,
	energy float64,
	peerEnergyMedian float64,
) float64 {
	return signal.Bounded(signal.Decoupling(correlation) * signal.EnergyLift(energy, peerEnergyMedian))
}

func (signal *Signal) NoiseScore(
	correlation float64,
	energy float64,
	peerEnergyMedian float64,
) float64 {
	energyLift := signal.EnergyLift(energy, peerEnergyMedian)

	return signal.Bounded(signal.Decoupling(correlation) / (1 + energyLift))
}

func (signal *Signal) StressScore(
	correlation float64,
	energy float64,
	peerEnergyMedian float64,
) float64 {
	return signal.Bounded(math.Max(0, -correlation) * signal.EnergyLift(energy, peerEnergyMedian))
}

func (signal *Signal) Decoupling(correlation float64) float64 {
	if math.IsNaN(correlation) || math.IsInf(correlation, 0) {
		return 0
	}

	return math.Max(0, 1-math.Abs(correlation))
}

func (signal *Signal) EnergyLift(energy float64, peerEnergyMedian float64) float64 {
	if peerEnergyMedian <= 0 {
		return 0
	}

	return energy / peerEnergyMedian
}

func (signal *Signal) Bounded(value float64) float64 {
	if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}

	return value / (1 + value)
}

func (signal *Signal) Positive(values []float64) []float64 {
	positive := make([]float64, 0, len(values))

	for _, value := range values {
		if value > 0 {
			positive = append(positive, value)
		}
	}

	return positive
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return signal.err
}
