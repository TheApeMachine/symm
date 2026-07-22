package correlation

import (
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/theapemachine/nomagique/algorithm"
	nomcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
)

/*
Section retains the price paths correlation actually needs and calculates each
updated symbol's relationship to its observed peers.
*/
type Section struct {
	samples        map[string][]nomcorrelation.Sample
	subjectReturns []float64
	peerReturns    []float64
	retention      []float64
}

/*
NewSection creates empty correlation history owned by one correlation signal.
*/
func NewSection() *Section {
	return &Section{
		samples: make(map[string][]nomcorrelation.Sample),
	}
}

/*
Measure records the current ticker batch and calculates correlation evidence
for every symbol updated by that batch.
*/
func (section *Section) Measure(
	rows []kraken.TickerData,
) (map[string]map[string]float64, error) {
	updated := make(map[string]struct{})

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() || row.Last == nil || row.Last.Sign() <= 0 {
			continue
		}

		samples := section.samples[symbol]

		if len(samples) > 0 && !row.Timestamp.After(samples[len(samples)-1].At) {
			continue
		}

		section.samples[symbol] = append(samples, nomcorrelation.Sample{
			At:    row.Timestamp,
			Value: row.Last.Float64(),
		})

		if err := section.trim(symbol); err != nil {
			return nil, err
		}

		updated[symbol] = struct{}{}
	}

	results := make(map[string]map[string]float64, len(updated))

	for symbol := range updated {
		scores, ok := section.scores(symbol)

		if ok {
			results[symbol] = scores
		}
	}

	return results, nil
}

/*
trim retains the adaptive long window derived from the symbol's observed
returns, keeping history bounded without imposing one market-wide horizon.
*/
func (section *Section) trim(symbol string) error {
	samples := section.samples[symbol]

	if len(samples) < 3 {
		return nil
	}

	section.retention = section.returns(samples, section.retention)
	_, longWindow, err := statistic.ResolveWindows(
		section.retention,
		0,
		0,
	)

	if err != nil {
		return fmt.Errorf("correlation: resolve %s retention: %w", symbol, err)
	}

	if longWindow <= 0 {
		return fmt.Errorf("correlation: %s retention must be positive", symbol)
	}

	if len(samples) <= longWindow+1 {
		return nil
	}

	section.samples[symbol] = samples[len(samples)-longWindow-1:]

	return nil
}

/*
scores calculates one symbol's cohort correlation and relative return energy.
*/
func (section *Section) scores(symbol string) (map[string]float64, bool) {
	subjectSamples := section.samples[symbol]

	if len(subjectSamples) < 3 {
		return nil, false
	}

	section.subjectReturns = section.returns(
		subjectSamples,
		section.subjectReturns,
	)

	var signedTotal float64
	var absoluteTotal float64
	var peerEnergyTotal float64
	var peerCount float64

	for peerSymbol, peerSamples := range section.samples {
		if peerSymbol == symbol || len(peerSamples) < 3 {
			continue
		}

		correlation, ok := section.correlation(subjectSamples, peerSamples)

		if !ok {
			continue
		}

		section.peerReturns = section.returns(peerSamples, section.peerReturns)

		signedTotal += correlation
		absoluteTotal += math.Abs(correlation)
		peerEnergyTotal += section.energy(section.peerReturns)
		peerCount++
	}

	if peerCount == 0 {
		return nil, false
	}

	signed := signedTotal / peerCount
	correlation := absoluteTotal / peerCount
	subjectEnergy := section.energy(section.subjectReturns)
	peerEnergy := peerEnergyTotal / peerCount

	if peerEnergy <= 0 {
		return nil, false
	}

	relativeEnergy := subjectEnergy / peerEnergy
	excessEnergy := math.Max(0, relativeEnergy-1)
	energyDeficit := math.Max(0, 1-relativeEnergy)
	excessMass := excessEnergy / (1 + excessEnergy)
	herdScore := math.Max(0, signed) / (1 + excessEnergy)
	alphaScore := excessMass / (1 + math.Max(0, signed))
	noiseScore := math.Max(0, 1-correlation) / (1 + excessEnergy + energyDeficit)
	stressScore := math.Max(0, -signed)
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

/*
correlation calculates the asynchronous return relationship between two price
paths.
*/
func (section *Section) correlation(
	left []nomcorrelation.Sample,
	right []nomcorrelation.Sample,
) (float64, bool) {
	return algorithm.HayashiPairCorrelation(left, right, 0)
}

/*
energy calculates median absolute return magnitude for scale-compatible peer
comparison.
*/
func (section *Section) energy(values []float64) float64 {
	median, ok := statistic.MedianAbsoluteOf(values)

	if !ok {
		return 0
	}

	return median
}

/*
returns calculates chronological log returns into reusable Section storage.
*/
func (section *Section) returns(
	samples []nomcorrelation.Sample,
	buffer []float64,
) []float64 {
	returns := slices.Grow(buffer[:0], len(samples)-1)[:len(samples)-1]
	written := 0

	for index := 1; index < len(samples); index++ {
		previous := samples[index-1].Value
		current := samples[index].Value

		if previous <= 0 || current <= 0 {
			continue
		}

		delta := samples[index].At.Sub(samples[index-1].At).Seconds()

		if delta <= 0 {
			continue
		}

		returns[written] = math.Log(current/previous) / math.Sqrt(delta)
		written++
	}

	return returns[:written]
}
