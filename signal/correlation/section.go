package correlation

import (
	"math"

	"github.com/theapemachine/nomagique/algorithm"
	nomcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/types"
)

/*
Section scores cohort correlation and relative energy for one symbol.
*/
type Section struct{}

func NewSection() *Section {
	return &Section{}
}

func (section *Section) Scores(
	symbol string,
	crossSection *types.CrossSection,
) (map[string]float64, bool) {
	if crossSection == nil {
		return nil, false
	}

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
		correlation, ok := section.correlation(subjectSamples, peerSamples)

		if !ok {
			continue
		}

		signedTotal += correlation
		absoluteTotal += math.Abs(correlation)
		peerEnergyTotal += section.energy(peerReturns)
		peerCount++
	}

	if peerCount == 0 {
		return nil, false
	}

	signed := signedTotal / peerCount
	correlation := absoluteTotal / peerCount
	subjectEnergy := section.energy(subject)
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

func (section *Section) correlation(
	left []nomcorrelation.Sample,
	right []nomcorrelation.Sample,
) (float64, bool) {
	if len(left) < 2 || len(right) < 2 {
		return 0, false
	}

	return algorithm.HayashiPairCorrelation(left, right, 0)
}

func (section *Section) energy(values []float64) float64 {
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
