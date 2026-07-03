package market

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
	"gonum.org/v1/gonum/stat"
)

type CrossSection struct {
	cfg             *datura.Artifact
	rows            map[string]*datura.Artifact
	symbols         []string
	breadths        []float64
	updateGaps      []float64
	positiveChanges int
	observedSymbols int
	volumes         []float64
	absChanges      map[string]float64
	version         int64
	PeerCache       *PeerCache
}

func CrossSectionConfigFromCadence(cadence float64) *datura.Artifact {
	window := 2

	if cadence > 0 {
		_, longWindow, err := statistic.ResolveWindows([]float64{cadence}, 0, 0)

		if err == nil && longWindow > window {
			window = longWindow
		}
	}

	minBars := max(window/8, 2)
	returnCap := max(window/4, 2)

	return datura.Acquire("market", datura.APPJSON).
		WithRole("cross_section_config").
		WithScope("peer").
		Poke(float64(returnCap), "return_cap").
		Poke(float64(minBars), "min_bars").
		Poke(float64(returnCap), "breadth_hist")
}

func DefaultCrossSectionConfig() *datura.Artifact {
	return CrossSectionConfigFromCadence(0)
}

func NewCrossSection(configs ...*datura.Artifact) (*CrossSection, error) {
	cfg := DefaultCrossSectionConfig()
	if len(configs) > 0 && configs[0] != nil {
		cfg = configs[0]
	}
	if datura.Peek[float64](cfg, "return_cap") < 2 {
		return nil, errnie.Error(fmt.Errorf("cross-section: return cap too small"))
	}
	if datura.Peek[float64](cfg, "min_bars") < 2 {
		return nil, errnie.Error(fmt.Errorf("cross-section: min bars too small"))
	}

	return &CrossSection{
		cfg:        cfg,
		rows:       make(map[string]*datura.Artifact),
		absChanges: make(map[string]float64),
		PeerCache:  NewPeerCache(),
	}, nil
}

func (crossSection *CrossSection) Observe(artifacts map[string][]*datura.Artifact) error {
	if crossSection == nil {
		return errnie.Error(fmt.Errorf("cross-section: nil receiver"))
	}

	var observedErr error

	for _, ticker := range artifacts["ticker"] {
		if ticker == nil {
			continue
		}

		if channel := datura.Peek[string](ticker, "channel"); channel != "" && channel != "ticker" {
			continue
		}

		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](ticker, "data", rowIndex, "symbol")
			if symbol == "" {
				break
			}

			if err := crossSection.observeTickerRow(ticker, rowIndex); err != nil {
				observedErr = errnie.Error(err)
			}
		}
	}

	return observedErr
}

func (crossSection *CrossSection) observeTickerRow(ticker *datura.Artifact, rowIndex int) error {
	name := datura.Peek[string](ticker, "data", rowIndex, "symbol")
	price := datura.Peek[float64](ticker, "data", rowIndex, "last")

	if name == "" {
		return fmt.Errorf("cross-section: empty symbol name")
	}
	if price <= 0 {
		return fmt.Errorf("cross-section: price must be positive")
	}
	if ticker.Timestamp() <= 0 {
		return fmt.Errorf("cross-section: ticker timestamp must be positive")
	}

	volume := datura.Peek[float64](ticker, "data", rowIndex, "volume")
	change := datura.Peek[float64](ticker, "data", rowIndex, "change_pct") / 100
	bidQty := datura.Peek[float64](ticker, "data", rowIndex, "bid_qty")
	askQty := datura.Peek[float64](ticker, "data", rowIndex, "ask_qty")
	updated := time.Unix(0, ticker.Timestamp())
	pressure := 0.0
	bookDepth := bidQty + askQty

	if bookDepth > 0 {
		pressure = (bidQty - askQty) / bookDepth
	}

	state := crossSection.ensure(name)
	priorUpdated := datura.Peek[int64](state, "updated")
	if priorUpdated > 0 {
		gap := updated.Sub(time.Unix(0, priorUpdated)).Seconds()
		if gap > 0 {
			crossSection.push(&crossSection.updateGaps, gap, crossSection.ReturnCap())
			crossSection.refreshConfig()
		}
	}

	returns := datura.Peek[[]float64](state, "returns")
	lastPrice := datura.Peek[float64](state, "price")
	if lastPrice > 0 {
		ret := math.Log(price / lastPrice)
		if ret != 0 || len(returns) == 0 {
			crossSection.push(&returns, ret, crossSection.ReturnCap())
		}
	}

	state.Poke(name, "symbol").
		Poke(returns, "returns").
		Poke(volume, "volume").
		Poke(pressure, "pressure").
		Poke(change, "change").
		Poke(price, "price").
		Poke(updated.UnixNano(), "updated")

	crossSection.version++
	crossSection.refreshAggregates()

	return nil
}

func (crossSection *CrossSection) ReturnCap() int {
	return max(int(datura.Peek[float64](crossSection.cfg, "return_cap")), 2)
}

func (crossSection *CrossSection) MinBarsRequired() int {
	return max(int(datura.Peek[float64](crossSection.cfg, "min_bars")), 2)
}

func (crossSection *CrossSection) MaxReturnWindow() int {
	return crossSection.ReturnCap()
}

func (crossSection *CrossSection) MedianCadence() float64 {
	median, _ := statistic.MedianOf(crossSection.updateGaps)

	return median
}

func (crossSection *CrossSection) SymbolReturns(name string, window int) []float64 {
	row := crossSection.rows[name]
	if row == nil {
		return nil
	}

	return crossSection.tail(datura.Peek[[]float64](row, "returns"), window)
}

func (crossSection *CrossSection) Breadth() float64 {
	if crossSection.observedSymbols == 0 {
		return 0
	}

	return float64(crossSection.positiveChanges) / float64(crossSection.observedSymbols)
}

func (crossSection *CrossSection) RecordBreadth(breadth float64) {
	crossSection.push(&crossSection.breadths, breadth, crossSection.breadthCap())
}

func (crossSection *CrossSection) BreadthCount() int {
	return len(crossSection.breadths)
}

func (crossSection *CrossSection) MajorityThreshold() float64 {
	if len(crossSection.breadths) == 0 {
		return crossSection.Breadth()
	}

	sorted := append([]float64(nil), crossSection.breadths...)
	sort.Float64s(sorted)

	return stat.Quantile(0.5, stat.LinInterp, sorted, nil)
}

func (crossSection *CrossSection) IsLeader(name string, change float64) bool {
	if change == 0 {
		return false
	}

	changes := crossSection.absoluteChanges(name, change)
	if len(changes) < 2 {
		return false
	}

	return math.Abs(change) >= crossSection.leadershipThreshold(changes)
}

func (crossSection *CrossSection) LeadershipThreshold() float64 {
	return crossSection.leadershipThreshold(crossSection.absoluteChanges("", 0))
}

func (crossSection *CrossSection) Volumes() []float64 {
	if len(crossSection.volumes) == 0 {
		return nil
	}

	return append([]float64(nil), crossSection.volumes...)
}

func (crossSection *CrossSection) DollarVolumes() []float64 {
	values := make([]float64, 0, len(crossSection.symbols))
	for _, symbol := range crossSection.symbols {
		row := crossSection.rows[symbol]
		if row == nil {
			continue
		}

		volume := datura.Peek[float64](row, "volume")
		price := datura.Peek[float64](row, "price")
		if volume > 0 && price > 0 {
			values = append(values, volume*price)
		}
	}

	return values
}

func (crossSection *CrossSection) Leader() string {
	if len(crossSection.absChanges) < 2 {
		return ""
	}

	changes := make([]float64, 0, len(crossSection.absChanges))
	leader := ""
	best := 0.0
	for symbol, absChange := range crossSection.absChanges {
		changes = append(changes, absChange)
		if absChange > best {
			leader = symbol
			best = absChange
		}
	}

	median, _ := statistic.MedianOf(changes)
	threshold := crossSection.leadershipThreshold(changes)
	if threshold-median <= 0 || best <= threshold {
		return ""
	}

	return leader
}

func (crossSection *CrossSection) Pressure(name string) float64 {
	row := crossSection.rows[name]
	if row == nil {
		return 0
	}

	return datura.Peek[float64](row, "pressure")
}

func (crossSection *CrossSection) Symbols() []string {
	symbols := make([]string, len(crossSection.symbols))
	copy(symbols, crossSection.symbols)

	return symbols
}

func (crossSection *CrossSection) ensure(name string) *datura.Artifact {
	if row := crossSection.rows[name]; row != nil {
		return row
	}

	row := datura.Acquire("market", datura.APPJSON).
		WithRole("cross_section").
		WithScope(name).
		Poke(name, "symbol").
		Poke([]float64{}, "returns")
	crossSection.rows[name] = row
	crossSection.symbols = append(crossSection.symbols, name)

	return row
}

func (crossSection *CrossSection) refreshAggregates() {
	positive := 0
	volumes := make([]float64, 0, len(crossSection.symbols))
	absChanges := make(map[string]float64, len(crossSection.symbols))

	for _, symbol := range crossSection.symbols {
		row := crossSection.rows[symbol]
		if row == nil {
			continue
		}

		change := datura.Peek[float64](row, "change")
		if change > 0 {
			positive++
		}
		if volume := datura.Peek[float64](row, "volume"); volume > 0 {
			volumes = append(volumes, volume)
		}
		absChanges[symbol] = math.Abs(change)
	}

	crossSection.positiveChanges = positive
	crossSection.observedSymbols = len(crossSection.symbols)
	crossSection.volumes = volumes
	crossSection.absChanges = absChanges
}

func (crossSection *CrossSection) refreshConfig() {
	if len(crossSection.updateGaps) == 0 {
		return
	}

	_, longWindow, err := statistic.ResolveWindows(crossSection.updateGaps, 0, 0)

	if err != nil {
		return
	}

	returnCap := max(crossSection.ReturnCap(), longWindow)
	minBars := max(crossSection.MinBarsRequired(), max(returnCap/4, 2))
	breadthHist := max(crossSection.breadthCap(), returnCap)

	crossSection.cfg = datura.Acquire("market", datura.APPJSON).
		WithRole("cross_section_config").
		WithScope("peer").
		Poke(float64(returnCap), "return_cap").
		Poke(float64(minBars), "min_bars").
		Poke(float64(breadthHist), "breadth_hist")
}

func (crossSection *CrossSection) breadthCap() int {
	return max(int(datura.Peek[float64](crossSection.cfg, "breadth_hist")), crossSection.ReturnCap())
}

func (crossSection *CrossSection) absoluteChanges(name string, change float64) []float64 {
	changes := make([]float64, 0, len(crossSection.absChanges))
	for symbol, absChange := range crossSection.absChanges {
		if symbol == name {
			changes = append(changes, math.Abs(change))
			continue
		}
		changes = append(changes, absChange)
	}

	return changes
}

func (crossSection *CrossSection) push(buffer *[]float64, value float64, capacity int) {
	if capacity < 1 {
		capacity = 1
	}

	*buffer = append(*buffer, value)
	if len(*buffer) > capacity {
		*buffer = (*buffer)[len(*buffer)-capacity:]
	}
}

func (crossSection *CrossSection) tail(values []float64, window int) []float64 {
	if len(values) == 0 || window <= 0 {
		return nil
	}
	if len(values) <= window {
		return append([]float64(nil), values...)
	}

	return append([]float64(nil), values[len(values)-window:]...)
}

func (crossSection *CrossSection) leadershipThreshold(changes []float64) float64 {
	if len(changes) < 2 {
		return 0
	}

	sorted := append([]float64(nil), changes...)
	sort.Float64s(sorted)
	median, _ := statistic.MedianOf(sorted)
	deviations := make([]float64, 0, len(sorted))
	for _, value := range sorted {
		deviations = append(deviations, math.Abs(value-median))
	}

	deviationMedian, _ := statistic.MedianOf(deviations)

	return median + deviationMedian
}
