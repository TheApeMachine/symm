package correlation

import (
	"fmt"
	"math"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alitto/pond/v2"
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
	scratch         *sync.Map
	revisionBuf     []pairRevision
	pool            pond.Pool
	group           pond.TaskGroup
	measureMu       sync.Mutex
	stateMu         sync.Mutex
	energyMu        sync.Mutex
}

/*
NewSection creates empty correlation history owned by one correlation signal.
*/
func NewSection() *Section {
	section := &Section{
		symbols: make(map[string]*symbolState),
		pairs:   make(map[string]map[string]*pairState),
		scratch: &sync.Map{},
		pool:    pond.NewPool(runtime.GOMAXPROCS(0)),
	}

	section.group = section.pool.NewGroup()
	return section
}

/*
Measure records the current ticker batch, updates streaming Hayashi state for
each changed symbol, and returns cohort scores without rescanning full peer
histories for unchanged pairs.
*/
func (section *Section) Measure(
	rows []kraken.TickerData,
) (map[string]map[string]float64, error) {
	section.measureMu.Lock()
	defer section.measureMu.Unlock()

	if section.symbols == nil {
		section.symbols = make(map[string]*symbolState)
	}

	if section.pairs == nil {
		section.pairs = make(map[string]map[string]*pairState)
	}

	if section.scratch == nil {
		section.scratch = &sync.Map{}
	}

	if section.pool == nil {
		section.pool = pond.NewPool(runtime.GOMAXPROCS(0))
	}

	if section.group == nil {
		section.group = section.pool.NewGroup()
	}

	section.scratch.Clear()
	rowBatches := make(map[string][]kraken.TickerData)
	changedSymbols := make([]string, 0)
	seenSymbols := make(map[string]struct{})
	errorsBySymbol := &sync.Map{}

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() || row.Last == nil || row.Last.Sign() <= 0 {
			continue
		}

		if _, exists := seenSymbols[symbol]; !exists {
			seenSymbols[symbol] = struct{}{}
			changedSymbols = append(changedSymbols, symbol)
		}

		rowBatches[symbol] = append(rowBatches[symbol], row)
	}
	sort.Strings(changedSymbols)

	for _, symbol := range changedSymbols {
		symbolRows := rowBatches[symbol]
		sort.SliceStable(symbolRows, func(leftIndex, rightIndex int) bool {
			return symbolRows[leftIndex].Timestamp.Before(symbolRows[rightIndex].Timestamp)
		})

		section.group.Submit(func() {
			state := section.ensure(symbol)

			for _, row := range symbolRows {
				if len(state.samples) > 0 && !row.Timestamp.After(state.samples[len(state.samples)-1].At) {
					continue
				}

				if err := section.appendSample(symbol, state, nomcorrelation.Sample{
					At:    row.Timestamp,
					Value: row.Last.Float64(),
				}); err != nil {
					errorsBySymbol.Store(symbol, err)
					return
				}
			}

			section.scratch.Store(symbol, struct{}{})
		})
	}

	if err := section.group.Wait(); err != nil {
		return nil, err
	}

	var measurementErr error
	errorsBySymbol.Range(func(key, value any) bool {
		measurementErr = value.(error)
		return false
	})

	if measurementErr != nil {
		return nil, measurementErr
	}

	empty := true

	section.scratch.Range(func(key, value any) bool {
		empty = false
		return false
	})

	if empty {
		return nil, nil
	}

	section.stateMu.Lock()
	allSymbols := make([]string, 0, len(section.symbols))

	for symbol := range section.symbols {
		allSymbols = append(allSymbols, symbol)
	}
	section.stateMu.Unlock()
	sort.Strings(allSymbols)
	concurrentResults := &sync.Map{}

	for _, symbol := range allSymbols {
		section.group.Submit(func() {
			scores, ok := section.scores(symbol)

			if ok {
				concurrentResults.Store(symbol, scores)
			}
		})
	}

	if err := section.group.Wait(); err != nil {
		return nil, err
	}

	results := make(map[string]map[string]float64, len(allSymbols))

	for _, symbol := range allSymbols {
		raw, exists := concurrentResults.Load(symbol)

		if exists {
			results[symbol] = raw.(map[string]float64)
		}
	}

	return results, nil
}

/*
LastAt returns the newest sample time retained for symbol.
Emission uses this for cohort peers that were scored but absent from the
current ticker batch so focus-gated UI still receives a live reading.
*/
func (section *Section) LastAt(symbol string) time.Time {
	state := section.symbols[symbol]

	if state == nil || len(state.samples) == 0 {
		return time.Time{}
	}

	return state.samples[len(state.samples)-1].At
}

/*
ensure returns the mutable state for symbol, creating it on first sight.
*/
func (section *Section) ensure(symbol string) *symbolState {
	section.stateMu.Lock()
	defer section.stateMu.Unlock()

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
	state.variance = exactVariance(state)

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
		state.samples = state.samples[1:]
		state.logPrices = state.logPrices[1:]
	}
	state.variance = exactVariance(state)

	return section.refreshEnergy(symbol, state)
}

/*
refreshEnergy recomputes time-normalized return energy and keeps the global
energy sum consistent for O(1) leave-one-out peer energy.
*/
func (section *Section) refreshEnergy(symbol string, state *symbolState) error {
	section.energyMu.Lock()
	defer section.energyMu.Unlock()

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

func (section *Section) Close() {
	section.measureMu.Lock()
	defer section.measureMu.Unlock()

	if section.pool != nil {
		section.pool.StopAndWait()
	}
}

/*
scores derives herd/alpha/noise/stress from streaming cohort aggregates.
*/
func (section *Section) scores(symbol string) (map[string]float64, bool) {
	state := section.symbols[symbol]

	if state == nil || state.energy <= 0 {
		return nil, false
	}

	weightedSigned := 0.0
	weightedAbsolute := 0.0
	weightedPeerEnergy := 0.0
	totalSupport := 0.0

	for peerSymbol, peer := range section.symbols {
		if peerSymbol == symbol || peer.energy <= 0 {
			continue
		}

		correlationValue, support, ok := supportedCorrelation(
			state.samples, peer.samples, state.logPrices, peer.logPrices,
		)

		if !ok {
			continue
		}

		weight := float64(support)
		weightedSigned += correlationValue * weight
		weightedAbsolute += math.Abs(correlationValue) * weight
		weightedPeerEnergy += peer.energy * weight
		totalSupport += weight
	}

	if totalSupport <= 0 {
		return nil, false
	}

	peerEnergy := weightedPeerEnergy / totalSupport

	if peerEnergy <= 0 {
		return nil, false
	}

	signed := weightedSigned / totalSupport
	correlation := weightedAbsolute / totalSupport
	relativeEnergy := state.energy / peerEnergy
	excessEnergy := math.Max(0, relativeEnergy-1)
	energyDeficit := math.Max(0, 1-relativeEnergy)
	excessMass := excessEnergy / (1 + excessEnergy)
	herdScore := math.Max(0, signed) / (1 + excessEnergy)
	alphaScore := excessMass / (1 + math.Max(0, signed))
	noiseScore := math.Max(0, 1-correlation) / (1 + excessEnergy + energyDeficit)
	stressScore := math.Max(0, -signed)
	strength := max(max(herdScore, alphaScore), max(noiseScore, stressScore))

	// Zero strength is still a live cohort reading (quiet / locked peers).
	// Suppressing it left the focused kernel on STANDBY forever in calm tapes.
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
