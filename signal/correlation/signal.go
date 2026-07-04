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
Signal measures whether a symbol is moving with the cohort, against it, beyond
it, or without a stable relation to it.
*/
type Signal struct {
	ctx        context.Context
	cancel     context.CancelFunc
	err        error
	tree       *dmt.Tree
	classifier *probability.ScoreClassifier
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		classifier: probability.NewScoreClassifier(
			[]string{"herdScore", "alphaScore", "noiseScore", "stressScore"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategorySystemicHerd)),
				float64(logic.CategoryIndex(logic.CategoryDecoupledAlpha)),
				float64(logic.CategoryIndex(logic.CategoryStochasticNoise)),
				float64(logic.CategoryIndex(logic.CategoryDivergentStress)),
			},
		),
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
		if crossSection == nil {
			yield(datapoint.WithError(errnie.Error(errnie.Err(
				errnie.Validation,
				"correlation: cross-section required",
				nil,
			))))
			return
		}

		if datura.Peek[string](datapoint, "role") != "ticker" {
			return
		}

		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")
			if symbol == "" {
				return
			}

			measurement := signal.measure(symbol, datapoint.Timestamp(), crossSection)
			if measurement == nil {
				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) measure(
	symbol string,
	timestamp int64,
	crossSection *market.CrossSection,
) *datura.Artifact {
	output, ok := signal.score(symbol, crossSection)
	if !ok {
		return nil
	}

	measurement := datura.Acquire("correlation", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceCorrelation)))
	measurement.SetTimestamp(timestamp)
	measurement.MergeOutputs(output)

	result, err := signal.classifier.Classify(map[string]float64{
		"herdScore":   output["herdScore"].(float64),
		"alphaScore":  output["alphaScore"].(float64),
		"noiseScore":  output["noiseScore"].(float64),
		"stressScore": output["stressScore"].(float64),
		"strength":    output["strength"].(float64),
	})
	if err != nil {
		return measurement.WithError(errnie.Error(err))
	}

	for key, value := range result.Outputs() {
		measurement.MergeOutput(key, value)
	}

	if datura.Peek[float64](measurement, "output", "confidence") <= 0 {
		measurement.Release()
		return nil
	}

	measurement.Poke("output", "root")

	return measurement
}

func (signal *Signal) score(
	symbol string,
	crossSection *market.CrossSection,
) (map[string]any, bool) {
	window := crossSection.MaxReturnWindow()
	subject := crossSection.SymbolReturns(symbol, window)
	if len(subject) < 2 {
		return nil, false
	}

	var signedTotal float64
	var absoluteTotal float64
	var peerEnergyTotal float64
	var peerCount float64

	for _, peer := range crossSection.Symbols() {
		if peer == symbol {
			continue
		}

		peerReturns := crossSection.SymbolReturns(peer, window)
		correlation, ok := signal.correlation(subject, peerReturns)
		if !ok {
			continue
		}

		signedTotal += correlation
		absoluteTotal += math.Abs(correlation)
		peerEnergyTotal += signal.energy(peerReturns)
		peerCount++
	}

	if peerCount == 0 {
		return nil, false
	}

	signed := signedTotal / peerCount
	correlation := absoluteTotal / peerCount
	subjectEnergy := signal.energy(subject)
	peerEnergy := peerEnergyTotal / peerCount
	if peerEnergy <= 0 {
		return nil, false
	}

	relativeEnergy := subjectEnergy / peerEnergy
	excessEnergy := math.Max(0, relativeEnergy-1)
	energyDeficit := math.Max(0, 1-relativeEnergy)
	herdScore := math.Max(0, signed) / (1 + excessEnergy)
	alphaScore := excessEnergy / (1 + math.Max(0, signed))
	noiseScore := math.Max(0, 1-correlation) / (1 + excessEnergy + energyDeficit)
	stressScore := math.Max(0, -signed) * (1 + excessEnergy)
	strength := max(max(herdScore, alphaScore), max(noiseScore, stressScore))

	return map[string]any{
		"correlation":    correlation,
		"signed":         signed,
		"relativeEnergy": relativeEnergy,
		"herdScore":      herdScore,
		"alphaScore":     alphaScore,
		"noiseScore":     noiseScore,
		"stressScore":    stressScore,
		"peakScore":      strength,
		"strength":       strength,
	}, true
}

func (signal *Signal) correlation(left []float64, right []float64) (float64, bool) {
	count := min(len(left), len(right))
	if count < 2 {
		return 0, false
	}

	left = left[len(left)-count:]
	right = right[len(right)-count:]
	leftMean := signal.mean(left)
	rightMean := signal.mean(right)
	var covariance float64
	var leftVariance float64
	var rightVariance float64

	for index := range count {
		leftDelta := left[index] - leftMean
		rightDelta := right[index] - rightMean
		covariance += leftDelta * rightDelta
		leftVariance += leftDelta * leftDelta
		rightVariance += rightDelta * rightDelta
	}

	denominator := math.Sqrt(leftVariance * rightVariance)
	if denominator <= 0 {
		return 0, false
	}

	return covariance / denominator, true
}

func (signal *Signal) energy(values []float64) float64 {
	absolute := make([]float64, 0, len(values))
	for _, value := range values {
		absolute = append(absolute, math.Abs(value))
	}

	median, ok := statistic.MedianOf(absolute)
	if !ok {
		return 0
	}

	return median
}

func (signal *Signal) mean(values []float64) float64 {
	var total float64
	for _, value := range values {
		total += value
	}

	return total / float64(len(values))
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
