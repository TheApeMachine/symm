package correlation

import (
	"fmt"
	"math"
	"strings"

	nomcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
)

/*
symbolState retains one symbol's price path, precomputed log-prices, Hayashi
variance, cohort energy, and running peer-correlation aggregates so Measure
never rescans the universe to rebuild those scalars.
*/
type symbolState struct {
	samples    []nomcorrelation.Sample
	logPrices  []float64
	returns    []float64
	energy     float64
	variance   float64
	longWindow int
	signedSum  float64
	absSum     float64
	peerCount  int
}

/*
pairState retains the Hayashi cross-covariance for one unordered symbol pair.
Correlation is derived from covariance and the two symbol variances.
*/
type pairState struct {
	covariance float64
}

/*
Section owns per-symbol streaming state and the nested pair covariance table
used for incremental Hayashi maintenance without composite string keys.
*/
type Section struct {
	symbols         map[string]*symbolState
	pairs           map[string]map[string]*pairState
	globalEnergySum float64
	energyReady     int
	scratch         []string
	revisionBuf     []pairRevision
}

/*
NewSection creates empty correlation history owned by one correlation signal.
*/
func NewSection() *Section {
	return &Section{
		symbols: make(map[string]*symbolState),
		pairs:   make(map[string]map[string]*pairState),
	}
}

/*
Measure records the current ticker batch, updates streaming Hayashi state for
each changed symbol, and returns cohort scores without rescanning full peer
histories for unchanged pairs.
*/
func (section *Section) Measure(
	rows []kraken.TickerData,
) (map[string]map[string]float64, error) {
	section.scratch = section.scratch[:0]

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() || row.Last == nil || row.Last.Sign() <= 0 {
			continue
		}

		state := section.ensure(symbol)

		if len(state.samples) > 0 && !row.Timestamp.After(state.samples[len(state.samples)-1].At) {
			continue
		}

		if err := section.appendSample(symbol, state, nomcorrelation.Sample{
			At:    row.Timestamp,
			Value: row.Last.Float64(),
		}); err != nil {
			return nil, err
		}

		section.scratch = append(section.scratch, symbol)
	}

	if len(section.scratch) == 0 {
		return nil, nil
	}

	updated := uniqueSymbols(section.scratch)
	results := make(map[string]map[string]float64, len(updated))

	for _, symbol := range updated {
		scores, ok := section.scores(symbol)

		if ok {
			results[symbol] = scores
		}
	}

	return results, nil
}

/*
ensure returns the mutable state for symbol, creating it on first sight.
*/
func (section *Section) ensure(symbol string) *symbolState {
	state := section.symbols[symbol]

	if state != nil {
		return state
	}

	state = &symbolState{}
	section.symbols[symbol] = state

	return state
}

/*
appendSample extends one symbol path, maintains variance and pair covariances
incrementally, then trims through the adaptive retention window.
*/
func (section *Section) appendSample(
	symbol string,
	state *symbolState,
	sample nomcorrelation.Sample,
) error {
	state.samples = append(state.samples, sample)
	state.logPrices = append(state.logPrices, math.Log(sample.Value))

	if len(state.samples) >= 2 {
		section.applyRightEdge(symbol, state)
	}

	if err := section.refreshEnergy(symbol, state); err != nil {
		return err
	}

	return section.trim(symbol, state)
}

/*
trim drops left-edge samples beyond the adaptive long window while reversing
their contribution to variance and pair covariances.
*/
func (section *Section) trim(symbol string, state *symbolState) error {
	if len(state.samples) < 3 {
		return nil
	}

	if state.longWindow == 0 || len(state.samples) > state.longWindow+1 {
		state.returns = fillReturns(state.samples, state.logPrices, state.returns)
		_, longWindow, err := statistic.ResolveWindows(state.returns, 0, 0)

		if err != nil {
			return fmt.Errorf("correlation: resolve %s retention: %w", symbol, err)
		}

		if longWindow <= 0 {
			return fmt.Errorf("correlation: %s retention must be positive", symbol)
		}

		state.longWindow = longWindow
	}

	for len(state.samples) > state.longWindow+1 {
		section.dropLeftEdge(symbol, state)
	}

	return section.refreshEnergy(symbol, state)
}

/*
refreshEnergy recomputes time-normalized return energy and keeps the global
energy sum consistent for O(1) leave-one-out peer energy.
*/
func (section *Section) refreshEnergy(symbol string, state *symbolState) error {
	previous := state.energy
	wasReady := previous > 0 && len(state.samples) >= 3

	if len(state.samples) < 3 {
		state.returns = state.returns[:0]
		state.energy = 0

		if wasReady {
			section.globalEnergySum -= previous
			section.energyReady--
		}

		return nil
	}

	state.returns = fillReturns(state.samples, state.logPrices, state.returns)
	median, ok := statistic.MedianAbsoluteOf(state.returns)

	if !ok {
		median = 0
	}

	state.energy = median
	ready := state.energy > 0

	if wasReady {
		section.globalEnergySum -= previous
		section.energyReady--
	}

	if ready {
		section.globalEnergySum += state.energy
		section.energyReady++
	}

	_ = symbol

	return nil
}

/*
scores derives herd/alpha/noise/stress from streaming cohort aggregates.
*/
func (section *Section) scores(symbol string) (map[string]float64, bool) {
	state := section.symbols[symbol]

	if state == nil || state.peerCount <= 0 || state.energy <= 0 {
		return nil, false
	}

	if section.energyReady <= 1 {
		return nil, false
	}

	peerEnergy := (section.globalEnergySum - state.energy) / float64(section.energyReady-1)

	if peerEnergy <= 0 {
		return nil, false
	}

	peerCount := float64(state.peerCount)
	signed := state.signedSum / peerCount
	correlation := state.absSum / peerCount
	relativeEnergy := state.energy / peerEnergy
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

func uniqueSymbols(symbols []string) []string {
	if len(symbols) < 2 {
		return symbols
	}

	seen := make(map[string]struct{}, len(symbols))
	out := symbols[:0]

	for _, symbol := range symbols {
		if _, ok := seen[symbol]; ok {
			continue
		}

		seen[symbol] = struct{}{}
		out = append(out, symbol)
	}

	return out
}

func fillReturns(
	samples []nomcorrelation.Sample,
	logPrices []float64,
	buffer []float64,
) []float64 {
	returns := buffer[:0]

	if cap(returns) < len(samples)-1 {
		returns = make([]float64, 0, len(samples)-1)
	}

	if len(logPrices) != len(samples) {
		return returns
	}

	for index := 1; index < len(samples); index++ {
		if samples[index-1].Value <= 0 || samples[index].Value <= 0 {
			continue
		}

		delta := samples[index].At.Sub(samples[index-1].At).Seconds()

		if delta <= 0 {
			continue
		}

		returns = append(
			returns,
			(logPrices[index]-logPrices[index-1])/math.Sqrt(delta),
		)
	}

	return returns
}
