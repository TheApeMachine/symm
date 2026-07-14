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
Correlation scores every peer in the cross-section for every subject
symbol every tick.

ponytail: this is O(symbols²) per tick; return/sample/absolute-value
buffers are held here and reused across calls instead of being
reallocated on each one. Upgrade path: pre-aggregate peer statistics
once per tick instead of rescoring pairwise if the symbol count grows.
*/
type Section struct {
	subjectReturns []float64
	subjectSamples []nomcorrelation.Sample
	peerReturns    []float64
	peerSamples    []nomcorrelation.Sample
	energyAbsolute []float64
}

func NewSection() *Section {
	return &Section{}
}

func ensureReturnLen(buffer []float64, length int) []float64 {
	if cap(buffer) >= length {
		return buffer[:length]
	}

	return make([]float64, length)
}

func ensureSampleLen(buffer []nomcorrelation.Sample, length int) []nomcorrelation.Sample {
	if cap(buffer) >= length {
		return buffer[:length]
	}

	return make([]nomcorrelation.Sample, length)
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

	section.subjectReturns = ensureReturnLen(section.subjectReturns, window)
	subjectWritten := crossSection.SymbolReturns(symbol, section.subjectReturns)

	if subjectWritten < 2 {
		return nil, false
	}

	section.subjectSamples = ensureSampleLen(section.subjectSamples, sampleWindow)
	subjectSampleWritten := crossSection.SymbolSamples(symbol, section.subjectSamples)

	if subjectSampleWritten < 2 {
		return nil, false
	}

	subjectSamples := section.subjectSamples[:subjectSampleWritten]

	section.peerReturns = ensureReturnLen(section.peerReturns, window)
	section.peerSamples = ensureSampleLen(section.peerSamples, sampleWindow)

	var signedTotal float64
	var absoluteTotal float64
	var peerEnergyTotal float64
	var peerCount float64

	for _, peer := range crossSection.ReadView().Metrics {
		if peer.Symbol == symbol {
			continue
		}

		peerWritten := crossSection.SymbolReturns(peer.Symbol, section.peerReturns)

		if peerWritten < 2 {
			continue
		}

		peerSampleWritten := crossSection.SymbolSamples(peer.Symbol, section.peerSamples)
		peerSamples := section.peerSamples[:peerSampleWritten]

		correlation, ok := section.correlation(subjectSamples, peerSamples)

		if !ok {
			continue
		}

		signedTotal += correlation
		absoluteTotal += math.Abs(correlation)
		peerEnergyTotal += section.energy(section.peerReturns[:peerWritten])
		peerCount++
	}

	if peerCount == 0 {
		return nil, false
	}

	signed := signedTotal / peerCount
	correlation := absoluteTotal / peerCount
	subjectEnergy := section.energy(section.subjectReturns[:subjectWritten])
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
	section.energyAbsolute = ensureReturnLen(section.energyAbsolute, len(values))

	for index, value := range values {
		section.energyAbsolute[index] = math.Abs(value)
	}

	median, ok := statistic.MedianOf(section.energyAbsolute)

	if !ok {
		return 0
	}

	return median
}
