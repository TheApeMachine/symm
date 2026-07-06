package correlation

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	nomcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	classifier *probability.ScoreClassifier
}

func NewTicker() *Ticker {
	return &Ticker{
		classifier: probability.NewScoreClassifier(
			[]string{"herdScore", "alphaScore", "noiseScore", "stressScore"},
			[]float64{
				float64(types.CategoryIndex(types.CategorySystemicHerd)),
				float64(types.CategoryIndex(types.CategoryDecoupledAlpha)),
				float64(types.CategoryIndex(types.CategoryStochasticNoise)),
				float64(types.CategoryIndex(types.CategoryDivergentStress)),
			},
		),
	}
}

func (ticker *Ticker) Measure(
	row kraken.TickerData,
	crossSection *types.CrossSection,
) ([]*types.Measurement, error) {
	if crossSection == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation, "correlation: cross-section required", nil,
		))
	}

	output, ok := ticker.score(row.Symbol, crossSection)

	if !ok {
		return nil, nil
	}

	result, err := ticker.classifier.Classify(map[string]float64{
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

	categories := []types.CategoryType{
		types.SystemicHerd,
		types.DecoupledAlpha,
		types.StochasticNoise,
		types.DivergentStress,
	}
	strengths := []float64{
		output["herdScore"],
		output["alphaScore"],
		output["noiseScore"],
		output["stressScore"],
	}
	categoryRows := make([]types.Category, 0, len(categories))

	for index, category := range categories {
		confidence := 0.0

		if index < len(result.Probabilities) {
			confidence = result.Probabilities[index]
		}

		categoryRows = append(categoryRows, types.Category{
			Type:       category,
			Confidence: confidence,
			Strength:   strengths[index],
		})
	}

	measurement := &types.Measurement{
		Source:        types.SourceCorrelation,
		Symbol:        row.Symbol,
		At:            row.Timestamp,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics:       output,
	}

	return []*types.Measurement{measurement}, nil
}

func (ticker *Ticker) score(
	symbol string,
	crossSection *types.CrossSection,
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
		correlation, ok := ticker.correlation(subjectSamples, peerSamples)

		if !ok {
			continue
		}

		signedTotal += correlation
		absoluteTotal += math.Abs(correlation)
		peerEnergyTotal += ticker.energy(peerReturns)
		peerCount++
	}

	if peerCount == 0 {
		return nil, false
	}

	signed := signedTotal / peerCount
	correlation := absoluteTotal / peerCount
	subjectEnergy := ticker.energy(subject)
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

	if strength <= 0 {
		return nil, false
	}

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

func (ticker *Ticker) correlation(
	left []nomcorrelation.Sample,
	right []nomcorrelation.Sample,
) (float64, bool) {
	if len(left) < 2 || len(right) < 2 {
		return 0, false
	}

	return algorithm.HayashiPairCorrelation(left, right, 0)
}

func (ticker *Ticker) energy(values []float64) float64 {
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
