package market

import (
	"container/ring"
	"fmt"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/config"
	market "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/nomagique/statistic"
)

type CrossSectionConfig struct {
	MatchWindow time.Duration
	ReturnCap   int
	MinBars     int
	BreadthHist int
}

func (cfg *CrossSectionConfig) Validate() error {
	return errnie.Error(errnie.Require(map[string]any{
		"match_window": cfg.MatchWindow,
		"return_cap":   cfg.ReturnCap,
		"min_bars":     cfg.MinBars,
		"breadth_hist": cfg.BreadthHist,
	}))
}

/*
CrossSection holds per-symbol cross-section state for signal measure paths.
*/
type CrossSection struct {
	universe    sync.Map
	matchWindow time.Duration
	returnCap   int
	minBars     int
	breadthHist *ring.Ring
}

/*
NewCrossSection builds cross-section state from signal config.
*/
func NewCrossSection(cfg *CrossSectionConfig) (cs *CrossSection, err error) {
	if cfg, err = config.NewSafeConfig(cfg); err != nil {
		return nil, errnie.Error(err)
	}

	return &CrossSection{
		matchWindow: cfg.MatchWindow,
		returnCap:   cfg.ReturnCap,
		minBars:     cfg.MinBars,
		breadthHist: ring.New(cfg.BreadthHist),
	}, nil
}

/*
CrossSectionOnce loads one shared cross-section per signal system.
*/
type CrossSectionOnce struct {
	value atomic.Pointer[crossSectionSlot]
}

type crossSectionSlot struct {
	cs  *CrossSection
	err error
}

func (loader *CrossSectionOnce) Load(cfg *CrossSectionConfig) (*CrossSection, error) {
	if loaded := loader.value.Load(); loaded != nil {
		return loaded.cs, loaded.err
	}

	cs, err := NewCrossSection(cfg)
	built := &crossSectionSlot{cs: cs, err: err}

	if loader.value.CompareAndSwap(nil, built) {
		return cs, err
	}

	loaded := loader.value.Load()

	return loaded.cs, loaded.err
}

/*
CrossSectionConfigFromViper reads validated cross-section config from viper.
*/
func CrossSectionConfigFromViper() (*CrossSectionConfig, error) {
	regime, err := config.DerivedRegimeSpec()

	if err != nil {
		return nil, errnie.Error(err)
	}

	derived := config.DerivedCrossSectionSpec(regime)

	return config.NewSafeConfig(&CrossSectionConfig{
		MatchWindow: derived.MatchWindow,
		ReturnCap:   derived.ReturnCap,
		MinBars:     derived.MinBars,
		BreadthHist: derived.BreadthHist,
	})
}

/*
LoadCrossSection builds the shared cross-section for a signal system.
*/
func LoadCrossSection(loader *CrossSectionOnce) (*CrossSection, error) {
	cfg, err := CrossSectionConfigFromViper()

	if err != nil {
		return nil, errnie.Error(err)
	}

	return loader.Load(cfg)
}

/*
Observe stores one complete symbol row in the cross-section universe.
*/
func (crossSection *CrossSection) Observe(src *market.Symbol) error {
	if err := src.Validate(); err != nil {
		return err
	}

	raw, _ := crossSection.universe.LoadOrStore(src.Name, &market.Symbol{Name: src.Name})
	dst := raw.(*market.Symbol)

	return dst.Update(src, crossSection.returnCap)
}

/*
Row returns a copy of the stored symbol row.
*/
func (crossSection *CrossSection) Row(name string) (market.Symbol, bool) {
	raw, ok := crossSection.universe.Load(name)

	if !ok {
		return market.Symbol{}, false
	}

	return *raw.(*market.Symbol), true
}

/*
MinBarsRequired is the minimum return history length for correlation reads.
*/
func (crossSection *CrossSection) MinBarsRequired() int {
	return crossSection.minBars
}

/*
SymbolReturns returns the trailing log-return window for one symbol.
*/
func (crossSection *CrossSection) SymbolReturns(symbol string, window int) []float64 {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return nil
	}

	row := raw.(*market.Symbol)

	if len(row.Returns) < window {
		return nil
	}

	return row.Returns[len(row.Returns)-window:]
}

/*
PeerWindowSnapshot holds cross-section peer stats from one universe scan.
*/
type PeerWindowSnapshot struct {
	MarketReturns    []float64
	PeerCorrelations []float64
	PeerEnergies     []float64
}

/*
PeerWindowSnapshot aligns peer return series and derives market returns,
correlations, and energies in one universe scan.
*/
func (crossSection *CrossSection) PeerWindowSnapshot(window int, at time.Time) PeerWindowSnapshot {
	series := make([][]float64, 0)

	crossSection.universe.Range(func(_, value any) bool {
		row := value.(*market.Symbol)

		if at.Sub(row.Updated) >= crossSection.matchWindow {
			return true
		}

		if len(row.Returns) < window {
			return true
		}

		series = append(series, row.Returns[len(row.Returns)-window:])
		return true
	})

	snapshot := PeerWindowSnapshot{}

	if len(series) < 2 {
		return snapshot
	}

	crossMarket := make([]float64, window)

	for index := range window {
		values := make([]float64, 0, len(series))

		for _, returns := range series {
			values = append(values, returns[index])
		}

		crossMarket[index] = float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(values...)...))
	}

	if len(crossMarket) < window {
		return snapshot
	}

	snapshot.MarketReturns = crossMarket
	snapshot.PeerCorrelations = make([]float64, 0, len(series))
	snapshot.PeerEnergies = make([]float64, 0, len(series))

	for _, returns := range series {
		snapshot.PeerCorrelations = append(snapshot.PeerCorrelations, float64(correlation.NewPearson(nil).Observe(
			append(
				nomagique.Numbers(returns...),
				nomagique.Numbers(crossMarket...)...,
			)...,
		)))
		snapshot.PeerEnergies = append(snapshot.PeerEnergies, float64(statistic.NewMedianAbsolute(nil).Observe(nomagique.Numbers(returns...)...)))
	}

	return snapshot
}

/*
MarketReturns aligns peer return series over a trailing window at event time.
*/
func (crossSection *CrossSection) MarketReturns(window int, at time.Time) []float64 {
	return crossSection.PeerWindowSnapshot(window, at).MarketReturns
}

/*
PeerCorrelations returns each fresh peer's correlation to cross-section market returns.
*/
func (crossSection *CrossSection) PeerCorrelations(window int, at time.Time) []float64 {
	return crossSection.PeerWindowSnapshot(window, at).PeerCorrelations
}

/*
PeerEnergies returns median absolute peer returns over a trailing window.
*/
func (crossSection *CrossSection) PeerEnergies(window int, at time.Time) []float64 {
	return crossSection.PeerWindowSnapshot(window, at).PeerEnergies
}

/*
MacroMomentum is the median peer change excluding the named symbol.
*/
func (crossSection *CrossSection) MacroMomentum(symbol string) float64 {
	changes := make([]float64, 0)

	crossSection.universe.Range(func(key, value any) bool {
		if key.(string) == symbol {
			return true
		}

		row := value.(*market.Symbol)

		if row.Value == 0 {
			return true
		}

		changes = append(changes, row.Value)
		return true
	})

	if len(changes) == 0 {
		return 0
	}

	return float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(changes...)...))
}

/*
Volumes returns quote volumes for all symbols with volume set.
*/
func (crossSection *CrossSection) Volumes() []float64 {
	out := make([]float64, 0)

	crossSection.universe.Range(func(_, value any) bool {
		row := value.(*market.Symbol)

		if row.Volume > 0 {
			out = append(out, row.Volume)
		}

		return true
	})

	return out
}

/*
TradePressure returns stored trade pressure for one symbol, or zero when the
symbol has not entered the universe yet.
*/
func (crossSection *CrossSection) TradePressure(symbol string) float64 {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return 0
	}

	return raw.(*market.Symbol).Pressure
}

/*
Pressure returns stored trade pressure for one symbol.
*/
func (crossSection *CrossSection) Pressure(symbol string) (float64, error) {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return 0, fmt.Errorf("cross_section: %s not in universe", symbol)
	}

	return raw.(*market.Symbol).Pressure, nil
}

/*
RecordBreadth appends a breadth sample to history.
*/
func (crossSection *CrossSection) RecordBreadth(breadth float64) {
	crossSection.breadthHist.Value = breadth
	crossSection.breadthHist = crossSection.breadthHist.Next()
}

/*
Breadth is the staleness-weighted fraction of symbols with positive change.
*/
func (crossSection *CrossSection) Breadth(at time.Time) float64 {
	positive := 0.0
	total := 0.0

	crossSection.universe.Range(func(_, value any) bool {
		row := value.(*market.Symbol)

		if row.Updated.IsZero() {
			return true
		}

		weight := crossSection.staleness(row.Updated, at)

		if weight <= 0 {
			return true
		}

		total += weight

		if row.Value > 0 {
			positive += weight
		}

		return true
	})

	if total <= 0 {
		return 0
	}

	breadth := positive / total

	if math.IsNaN(breadth) || math.IsInf(breadth, 0) {
		return 0
	}

	return breadth
}

/*
MajorityThreshold is the weighted fresh-symbol fraction required for a majority.
*/
func (crossSection *CrossSection) MajorityThreshold(at time.Time) float64 {
	freshCount := 0

	crossSection.universe.Range(func(_, value any) bool {
		row := value.(*market.Symbol)

		if row.Updated.IsZero() {
			return true
		}

		if crossSection.staleness(row.Updated, at) <= 0 {
			return true
		}

		freshCount++
		return true
	})

	if freshCount <= 0 {
		return 1
	}

	return float64(freshCount/2+1) / float64(freshCount)
}

/*
IsLeader reports whether the weighted change score ties or beats all peers.
*/
func (crossSection *CrossSection) IsLeader(symbol string, change float64, at time.Time) bool {
	if change <= 0 {
		return false
	}

	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return false
	}

	row := raw.(*market.Symbol)
	ownWeight := crossSection.staleness(row.Updated, at)

	if ownWeight <= 0 {
		return false
	}

	leaderScore := change * ownWeight
	best := 0.0

	crossSection.universe.Range(func(_, value any) bool {
		peer := value.(*market.Symbol)

		if peer.Updated.IsZero() {
			return true
		}

		weight := crossSection.staleness(peer.Updated, at)

		if weight <= 0 {
			return true
		}

		best = math.Max(best, peer.Value*weight)
		return true
	})

	return leaderScore >= best
}

func (crossSection *CrossSection) staleness(updatedAt, at time.Time) float64 {
	if updatedAt.IsZero() {
		return 0
	}

	elapsed := at.Sub(updatedAt)

	if elapsed < 0 {
		return 0
	}

	if crossSection.matchWindow <= 0 {
		return 1
	}

	if elapsed >= crossSection.matchWindow {
		return 0
	}

	return math.Exp(-float64(elapsed) / float64(crossSection.matchWindow))
}

/*
marketMedianReturns builds one aligned cross-section return series from symbols
with at least minSamples trailing returns. One median per lag — one regime read.
*/
func (crossSection *CrossSection) marketMedianReturns(
	at time.Time, window int, minSamples int,
) []float64 {
	tails := make([][]float64, 0)

	crossSection.universe.Range(func(_, value any) bool {
		row := value.(*market.Symbol)

		if crossSection.matchWindow > 0 && at.Sub(row.Updated) >= crossSection.matchWindow {
			return true
		}

		returns := crossSection.trailingSymbolReturns(row.Name, window)

		if len(returns) < minSamples {
			return true
		}

		tails = append(tails, returns[len(returns)-minSamples:])
		return true
	})

	if len(tails) == 0 {
		return nil
	}

	out := make([]float64, minSamples)

	for index := range minSamples {
		values := make([]float64, 0, len(tails))

		for _, tail := range tails {
			values = append(values, tail[index])
		}

		out[index] = float64(statistic.NewMedian(nil).Observe(nomagique.Numbers(values...)...))
	}

	return out
}

func (crossSection *CrossSection) trailingSymbolReturns(symbol string, window int) []float64 {
	raw, ok := crossSection.universe.Load(symbol)

	if !ok {
		return nil
	}

	row := raw.(*market.Symbol)

	if len(row.Returns) == 0 {
		return nil
	}

	sampleCount := window

	if sampleCount <= 0 || sampleCount > len(row.Returns) {
		sampleCount = len(row.Returns)
	}

	return row.Returns[len(row.Returns)-sampleCount:]
}
