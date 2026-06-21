package market

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/stat"
)

/*
CrossSectionConfig sizes peer analytics buffers.
*/
type CrossSectionConfig struct {
	MatchWindow time.Duration
	ReturnCap   int
	MinBars     int
	BreadthHist int
}

/*
PeerSnapshot holds peer-relative return analytics for one measurement window.
*/
type PeerSnapshot struct {
	MarketReturns    []float64
	PeerCorrelations []float64
	PeerEnergies     []float64
}

/*
CrossSectionOnce lazily loads one shared cross-section instance.
*/
type CrossSectionOnce struct {
	once    sync.Once
	section *CrossSection
	err     error
}

/*
LoadCrossSection returns a process-wide cross section from config defaults.
*/
func LoadCrossSection(loader *CrossSectionOnce) (*CrossSection, error) {
	loader.once.Do(func() {
		loader.section, loader.err = NewCrossSection(&CrossSectionConfig{
			MatchWindow: time.Minute,
			ReturnCap:   16,
			MinBars:     4,
			BreadthHist: 16,
		})
	})

	return loader.section, loader.err
}

type symbolState struct {
	returns    []float64
	volume     float64
	pressure   float64
	lastChange float64
	lastPrice  float64
	updated    time.Time
}

/*
CrossSection tracks return histories across the live symbol universe.
*/
type CrossSection struct {
	cfg      CrossSectionConfig
	universe sync.Map
	breadths []float64
}

/*
NewCrossSection allocates a cross-section store.
*/
func NewCrossSection(cfg *CrossSectionConfig) (*CrossSection, error) {
	if cfg == nil {
		return nil, errnie.Error(fmt.Errorf("cross-section: nil config"))
	}

	if cfg.ReturnCap < 4 {
		return nil, errnie.Error(fmt.Errorf("cross-section: return cap too small"))
	}

	if cfg.MinBars < 2 {
		return nil, errnie.Error(fmt.Errorf("cross-section: min bars too small"))
	}

	return &CrossSection{cfg: *cfg}, nil
}

func (crossSection *CrossSection) ensure(name string) *symbolState {
	raw, _ := crossSection.universe.LoadOrStore(name, &symbolState{})

	state, ok := raw.(*symbolState)

	if !ok {
		return nil
	}

	return state
}

/*
Observe appends one validated symbol row to the cross section.
*/
func (crossSection *CrossSection) Observe(row *Symbol) error {
	if row == nil {
		return errnie.Error(fmt.Errorf("cross-section: nil row"))
	}

	if err := row.Validate(); err != nil {
		return err
	}

	state := crossSection.ensure(row.Name)

	if state == nil {
		return errnie.Error(fmt.Errorf("cross-section: invalid state"))
	}

	ret := row.Value

	if state.lastPrice > 0 {
		ret = math.Log(row.Price / state.lastPrice)
	}

	if ret != 0 || len(state.returns) == 0 {
		pushCapped(&state.returns, ret, crossSection.cfg.ReturnCap)
	}

	state.volume = row.Volume
	state.pressure = row.Pressure
	state.lastChange = row.Value
	state.lastPrice = row.Price
	state.updated = row.Updated

	return nil
}

/*
MinBarsRequired returns the minimum return window for peer scoring.
*/
func (crossSection *CrossSection) MinBarsRequired() int {
	return crossSection.cfg.MinBars
}

/*
SymbolReturns returns the trailing return window for one symbol.
*/
func (crossSection *CrossSection) SymbolReturns(name string, window int) []float64 {
	raw, ok := crossSection.universe.Load(name)

	if !ok {
		return nil
	}

	state, ok := raw.(*symbolState)

	if !ok {
		return nil
	}

	return tailCopy(state.returns, window)
}

/*
PeerWindowSnapshot builds market and peer return analytics for the window.
*/
func (crossSection *CrossSection) PeerWindowSnapshot(
	window int,
	_ time.Time,
) PeerSnapshot {
	type peerSeries struct {
		name    string
		returns []float64
	}

	series := make([]peerSeries, 0)

	crossSection.universe.Range(func(key, value any) bool {
		state, ok := value.(*symbolState)

		if !ok {
			return true
		}

		returns := tailCopy(state.returns, window)

		if len(returns) < window {
			return true
		}

		series = append(series, peerSeries{
			name:    key.(string),
			returns: returns,
		})

		return true
	})

	if len(series) < 2 {
		return PeerSnapshot{}
	}

	marketReturns := make([]float64, window)

	for index := range window {
		column := make([]float64, len(series))

		for peerIndex, peer := range series {
			column[peerIndex] = peer.returns[index]
		}

		sort.Float64s(column)
		marketReturns[index] = stat.Quantile(0.5, stat.LinInterp, column, nil)
	}

	peerCorrelations := make([]float64, len(series))
	peerEnergies := make([]float64, len(series))

	for index, peer := range series {
		peerCorrelations[index] = stat.Correlation(peer.returns, marketReturns, nil)
		peerEnergies[index] = medianAbsolute(peer.returns)
	}

	return PeerSnapshot{
		MarketReturns:    marketReturns,
		PeerCorrelations: peerCorrelations,
		PeerEnergies:     peerEnergies,
	}
}

/*
Breadth returns the fraction of symbols with positive last change at or before at.
*/
func (crossSection *CrossSection) Breadth(at time.Time) float64 {
	positive := 0
	total := 0

	crossSection.universe.Range(func(_, value any) bool {
		state, ok := value.(*symbolState)

		if !ok {
			return true
		}

		if !state.updated.IsZero() && state.updated.After(at) {
			return true
		}

		total++

		if state.lastChange > 0 {
			positive++
		}

		return true
	})

	if total == 0 {
		return 0
	}

	return float64(positive) / float64(total)
}

/*
RecordBreadth stores one breadth sample for threshold derivation.
*/
func (crossSection *CrossSection) RecordBreadth(breadth float64) {
	pushCapped(&crossSection.breadths, breadth, crossSection.cfg.BreadthHist)
}

/*
MajorityThreshold returns the rolling median breadth.
*/
func (crossSection *CrossSection) MajorityThreshold(_ time.Time) float64 {
	if len(crossSection.breadths) == 0 {
		return 0.5
	}

	sorted := append([]float64(nil), crossSection.breadths...)
	sort.Float64s(sorted)

	return stat.Quantile(0.5, stat.LinInterp, sorted, nil)
}

/*
IsLeader reports whether the symbol change ranks in the top quartile at or before at.
*/
func (crossSection *CrossSection) IsLeader(name string, change float64, at time.Time) bool {
	if change == 0 {
		return false
	}

	changes := make([]float64, 0)

	crossSection.universe.Range(func(key, value any) bool {
		state, ok := value.(*symbolState)

		if !ok {
			return true
		}

		if !state.updated.IsZero() && state.updated.After(at) {
			return true
		}

		if key.(string) == name {
			changes = append(changes, math.Abs(change))

			return true
		}

		changes = append(changes, math.Abs(state.lastChange))

		return true
	})

	if len(changes) < 2 {
		return false
	}

	sort.Float64s(changes)
	threshold := stat.Quantile(0.75, stat.LinInterp, changes, nil)

	return math.Abs(change) >= threshold
}

/*
Volumes returns the latest quote volume per symbol.
*/
func (crossSection *CrossSection) Volumes() []float64 {
	volumes := make([]float64, 0)

	crossSection.universe.Range(func(_, value any) bool {
		state, ok := value.(*symbolState)

		if !ok || state.volume <= 0 {
			return true
		}

		volumes = append(volumes, state.volume)

		return true
	})

	return volumes
}

/*
Pressure returns the latest pressure observation for one symbol.
*/
func (crossSection *CrossSection) Pressure(name string) float64 {
	raw, ok := crossSection.universe.Load(name)

	if !ok {
		return 0
	}

	state, ok := raw.(*symbolState)

	if !ok {
		return 0
	}

	return state.pressure
}

/*
TradePressure returns the latest pressure observation for one symbol.
*/
func (crossSection *CrossSection) TradePressure(name string) float64 {
	return crossSection.Pressure(name)
}

func pushCapped(buffer *[]float64, value float64, capacity int) {
	*buffer = append(*buffer, value)

	if len(*buffer) > capacity {
		*buffer = (*buffer)[len(*buffer)-capacity:]
	}
}

func tailCopy(values []float64, window int) []float64 {
	if len(values) == 0 || window <= 0 {
		return nil
	}

	if len(values) <= window {
		out := make([]float64, len(values))
		copy(out, values)

		return out
	}

	out := make([]float64, window)
	copy(out, values[len(values)-window:])

	return out
}

func medianAbsolute(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	absValues := make([]float64, len(values))

	for index, value := range values {
		absValues[index] = math.Abs(value)
	}

	sort.Float64s(absValues)

	return stat.Quantile(0.5, stat.LinInterp, absValues, nil)
}
