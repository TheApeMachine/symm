package correlation

import (
	"context"
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	nomcorrelation "github.com/theapemachine/nomagique/correlation"
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
	classifier *probability.ScoreClassifier
}

func NewSignal(ctx context.Context) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
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
	input market.Input,
	crossSection *market.CrossSection,
) ([]*logic.Measurement, error) {
	if crossSection == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "correlation: cross-section required", nil,
		))
	}

	if input.Role != "ticker" {
		return nil, nil
	}

	measurements := make([]*logic.Measurement, 0, len(input.Ticker))
	for _, ticker := range input.Ticker {
		measurement, err := signal.measure(ticker.Symbol, ticker.Timestamp, crossSection)
		if err != nil {
			return nil, errnie.Error(errnie.Err(
				errnie.UnprocessableContent, err.Error(), err,
			))
		}

		if measurement == nil {
			continue
		}

		measurements = append(measurements, measurement)
	}

	return measurements, nil
}

func (signal *Signal) measure(
	symbol string,
	at time.Time,
	crossSection *market.CrossSection,
) (*logic.Measurement, error) {
	output, ok := signal.score(symbol, crossSection)
	if !ok {
		return nil, nil
	}

	result, err := signal.classifier.Classify(map[string]float64{
		"herdScore":   output["herdScore"],
		"alphaScore":  output["alphaScore"],
		"noiseScore":  output["noiseScore"],
		"stressScore": output["stressScore"],
		"strength":    output["strength"],
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	measurement := logic.NewMeasurement(logic.SourceCorrelation, symbol, at)

	for key, value := range output {
		measurement.AddMetric(key, value)
	}

	if err := measurement.ApplyClassifier(
		result.Value,
		result.Confidence,
		result.EntryBaseline,
		result.ExitBaseline,
		result.Strength,
		result.Distribution,
	); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if err := measurement.Ready(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	return measurement, nil
}

func (signal *Signal) score(
	symbol string,
	crossSection *market.CrossSection,
) (map[string]float64, bool) {
	window := crossSection.MaxReturnWindow()
	sampleWindow := window + 1
	subject := crossSection.SymbolReturns(symbol, window)
	if len(subject) < 2 {
		return nil, false
	}

	subjectSamples := crossSection.SymbolSamples(symbol, sampleWindow)
	if len(subjectSamples) < 2 {
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
		if len(peerReturns) < 2 {
			continue
		}

		peerSamples := crossSection.SymbolSamples(peer, sampleWindow)
		correlation, ok := signal.correlation(subjectSamples, peerSamples)
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

	return map[string]float64{
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

func (signal *Signal) correlation(
	left []nomcorrelation.Sample,
	right []nomcorrelation.Sample,
) (float64, bool) {
	if len(left) < 2 || len(right) < 2 {
		return 0, false
	}

	return algorithm.HayashiPairCorrelation(left, right, 0)
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

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
