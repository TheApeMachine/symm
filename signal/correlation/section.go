package correlation

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	nomcorrelation "github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
	"golang.org/x/sync/errgroup"
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
Section owns per-symbol streaming state and the nested pair covariance table
used for incremental Hayashi maintenance without composite string keys.
*/
type Section struct {
	symbols     *sync.Map
	pairs       *sync.Map
	scratch     *sync.Map
	revisionBuf []pairRevision
}

/*
NewSection creates empty correlation history owned by one correlation signal.
*/
func NewSection() *Section {
	section := &Section{
		symbols: &sync.Map{},
		pairs:   &sync.Map{},
		scratch: &sync.Map{},
	}

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
	if section.symbols == nil {
		section.symbols = &sync.Map{}
	}

	if section.pairs == nil {
		section.pairs = &sync.Map{}
	}

	if section.scratch == nil {
		section.scratch = &sync.Map{}
	}

	group, _ := errgroup.WithContext(context.Background())

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

		group.Go(func() error {
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
					return err
				}
			}

			section.scratch.Store(symbol, struct{}{})
			return nil
		})
	}

	if err := group.Wait(); err != nil {
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

	allSymbols := make([]string, 0)
	section.symbols.Range(func(key, value any) bool {
		allSymbols = append(allSymbols, key.(string))
		return true
	})

	sort.Strings(allSymbols)
	concurrentResults := &sync.Map{}

	for _, symbol := range allSymbols {
		group.Go(func() error {
			scores, ok, err := section.scores(symbol)
			if err != nil {
				return err
			}

			if ok {
				concurrentResults.Store(symbol, scores)
			}

			return nil
		})
	}

	if err := group.Wait(); err != nil {
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
	raw, _ := section.symbols.Load(symbol)
	state := raw.(*symbolState)

	if state == nil || len(state.samples) == 0 {
		return time.Time{}
	}

	return state.samples[len(state.samples)-1].At
}

/*
ensure returns the mutable state for symbol, creating it on first sight.
*/
func (section *Section) ensure(symbol string) *symbolState {
	raw, _ := section.symbols.LoadOrStore(symbol, &symbolState{})

	return raw.(*symbolState)
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
refreshEnergy recomputes time-normalized return energy for one symbol.
*/
func (section *Section) refreshEnergy(symbol string, state *symbolState) error {
	if len(state.samples) < 3 {
		state.returns = state.returns[:0]
		state.energy = 0

		return nil
	}

	state.returns = fillReturns(state.samples, state.logPrices, state.returns)
	median, ok := statistic.MedianAbsoluteOf(state.returns)

	if !ok {
		return fmt.Errorf("correlation: %s return-energy median unavailable", symbol)
	}

	state.energy = median

	return nil
}

func (section *Section) Close() {
}

/*
scores derives herd/alpha/noise/stress from streaming cohort aggregates.
*/
func (section *Section) scores(
	symbol string,
) (map[string]float64, bool, error) {
	raw, _ := section.symbols.Load(symbol)
	state := raw.(*symbolState)

	if state == nil || state.energy <= 0 {
		return nil, false, nil
	}

	weightedSigned := 0.0
	weightedAbsolute := 0.0
	weightedPeerEnergy := 0.0
	totalSupport := 0.0

	section.symbols.Range(func(key, value any) bool {
		peerSymbol := key.(string)
		peer := value.(*symbolState)

		if peerSymbol == symbol || peer.energy <= 0 {
			return true
		}

		correlationValue, support, ok := supportedCorrelation(
			state.samples, peer.samples, state.logPrices, peer.logPrices,
		)

		if !ok {
			return true
		}

		weight := float64(support)
		weightedSigned += correlationValue * weight
		weightedAbsolute += math.Abs(correlationValue) * weight
		weightedPeerEnergy += peer.energy * weight
		totalSupport += weight
		return true
	})

	if totalSupport <= 0 {
		return nil, false, nil
	}

	peerEnergy := weightedPeerEnergy / totalSupport

	if peerEnergy <= 0 {
		return nil, false, nil
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
	// A zero score bundle is still a live cohort reading (quiet / locked peers).
	// Suppressing it left the focused kernel on STANDBY forever in calm tapes.
	return map[string]float64{
		"correlation":    correlation,
		"signed":         signed,
		"relativeEnergy": relativeEnergy,
		"herdScore":      herdScore,
		"alphaScore":     alphaScore,
		"noiseScore":     noiseScore,
		"stressScore":    stressScore,
	}, true, nil
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
