package market

import (
	"container/ring"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	market "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/numeric"
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
	once sync.Once
	cs   *CrossSection
	err  error
}

func (loader *CrossSectionOnce) Load(cfg *CrossSectionConfig) (*CrossSection, error) {
	loader.once.Do(func() {
		loader.cs, loader.err = NewCrossSection(cfg)
	})

	return loader.cs, loader.err
}

/*
CrossSectionConfigFromViper reads validated cross-section config from viper.
*/
func CrossSectionConfigFromViper() (*CrossSectionConfig, error) {
	return config.NewSafeConfig(&CrossSectionConfig{
		MatchWindow: viper.GetDuration("signals.trade_match_window"),
		ReturnCap:   viper.GetInt("signals.cross_section.return_capacity"),
		MinBars:     viper.GetInt("signals.cross_section.min_bars"),
		BreadthHist: viper.GetInt("signals.cross_section.breadth_history_capacity"),
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
Observe merges a partial symbol update into the universe.
*/
func (crossSection *CrossSection) Observe(src *market.Symbol) {
	raw, _ := crossSection.universe.LoadOrStore(src.Name, &market.Symbol{Name: src.Name})
	dst := raw.(*market.Symbol)
	dst.Update(src, crossSection.returnCap)
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
MarketReturns aligns peer return series over a trailing window at event time.
*/
func (crossSection *CrossSection) MarketReturns(window int, at time.Time) []float64 {
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

	if len(series) < 2 {
		return nil
	}

	crossMarket := make([]float64, window)

	for index := range window {
		values := make([]float64, 0, len(series))

		for _, returns := range series {
			values = append(values, returns[index])
		}

		crossMarket[index] = numeric.Median(values)
	}

	return crossMarket
}

/*
PeerCorrelations returns each fresh peer's correlation to cross-section market returns.
*/
func (crossSection *CrossSection) PeerCorrelations(window int, at time.Time) []float64 {
	crossMarket := crossSection.MarketReturns(window, at)

	if len(crossMarket) < window {
		return nil
	}

	out := make([]float64, 0)

	crossSection.universe.Range(func(key, value any) bool {
		row := value.(*market.Symbol)

		if at.Sub(row.Updated) >= crossSection.matchWindow {
			return true
		}

		returns := crossSection.SymbolReturns(key.(string), window)

		if len(returns) < window {
			return true
		}

		out = append(out, numeric.Pearson(returns, crossMarket))
		return true
	})

	return out
}

/*
PeerEnergies returns median absolute peer returns over a trailing window.
*/
func (crossSection *CrossSection) PeerEnergies(window int, at time.Time) []float64 {
	out := make([]float64, 0)

	crossSection.universe.Range(func(_, value any) bool {
		row := value.(*market.Symbol)

		if at.Sub(row.Updated) >= crossSection.matchWindow {
			return true
		}

		if len(row.Returns) < window {
			return true
		}

		slice := row.Returns[len(row.Returns)-window:]
		out = append(out, numeric.MedianAbsolute(slice))

		return true
	})

	return out
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

	return numeric.Median(changes)
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

	return positive / total
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
	elapsed := at.Sub(updatedAt)

	if elapsed >= crossSection.matchWindow {
		return 0
	}

	return math.Exp(-float64(elapsed) / float64(crossSection.matchWindow))
}

func (crossSection *CrossSection) eachSymbolReturns(
	window int,
	visit func(symbol string, returns []float64),
) {
	if visit == nil {
		return
	}

	crossSection.universe.Range(func(_, value any) bool {
		symbolState, ok := value.(*market.Symbol)

		if !ok || symbolState == nil {
			return true
		}

		returns := crossSection.trailingSymbolReturns(symbolState.Name, window)

		if len(returns) == 0 {
			return true
		}

		visit(symbolState.Name, returns)

		return true
	})
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
