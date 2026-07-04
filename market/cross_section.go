package market

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/statistic"
	"github.com/theapemachine/symm/kraken"
	"gonum.org/v1/gonum/stat"
)

type CrossSectionConfig struct {
	ReturnCap  int
	MinBars    int
	BreadthCap int
}

type CrossSectionRow struct {
	Symbol   string
	Returns  []float64
	Volume   float64
	Pressure float64
	Change   float64
	Price    float64
	Updated  time.Time
}

type CrossSection struct {
	cfg             CrossSectionConfig
	rows            map[string]*CrossSectionRow
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

func CrossSectionConfigFromCadence(cadence float64) CrossSectionConfig {
	window := 2

	if cadence > 0 {
		_, longWindow, err := statistic.ResolveWindows([]float64{cadence}, 0, 0)
		if err == nil && longWindow > window {
			window = longWindow
		}
	}

	return CrossSectionConfig{
		ReturnCap:  max(window/4, 2),
		MinBars:    max(window/8, 2),
		BreadthCap: max(window/4, 2),
	}
}

func DefaultCrossSectionConfig() CrossSectionConfig {
	return CrossSectionConfigFromCadence(0)
}

func NewCrossSection(configs ...CrossSectionConfig) (*CrossSection, error) {
	cfg := DefaultCrossSectionConfig()
	if len(configs) > 0 {
		cfg = configs[0]
	}

	if cfg.ReturnCap < 2 {
		return nil, errnie.Error(fmt.Errorf("cross-section: return cap too small"))
	}

	if cfg.MinBars < 2 {
		return nil, errnie.Error(fmt.Errorf("cross-section: min bars too small"))
	}

	if cfg.BreadthCap < 2 {
		cfg.BreadthCap = cfg.ReturnCap
	}

	return &CrossSection{
		cfg:        cfg,
		rows:       make(map[string]*CrossSectionRow),
		absChanges: make(map[string]float64),
		PeerCache:  NewPeerCache(),
	}, nil
}

func (crossSection *CrossSection) Observe(tickers kraken.TickerDataSlice) error {
	if crossSection == nil {
		return errnie.Error(fmt.Errorf("cross-section: nil receiver"))
	}

	var observedErr error
	for _, ticker := range tickers {
		if ticker.Symbol == "" {
			observedErr = errnie.Error(fmt.Errorf("cross-section: empty symbol name"))
			continue
		}

		if ticker.Last <= 0 || math.IsNaN(ticker.Last) || math.IsInf(ticker.Last, 0) {
			continue
		}

		if ticker.Timestamp.IsZero() {
			observedErr = errnie.Error(fmt.Errorf("cross-section: ticker timestamp required"))
			continue
		}

		if err := crossSection.observeTickerRow(ticker); err != nil {
			observedErr = errnie.Error(err)
		}
	}

	return observedErr
}

func (crossSection *CrossSection) observeTickerRow(ticker kraken.TickerData) error {
	row := crossSection.ensure(ticker.Symbol)
	if !row.Updated.IsZero() {
		gap := ticker.Timestamp.Sub(row.Updated).Seconds()
		if gap > 0 {
			crossSection.push(&crossSection.updateGaps, gap, crossSection.ReturnCap())
			crossSection.refreshConfig()
		}
	}

	if row.Price > 0 {
		ret := math.Log(ticker.Last / row.Price)
		if ret != 0 || len(row.Returns) == 0 {
			crossSection.push(&row.Returns, ret, crossSection.ReturnCap())
		}
	}

	bookDepth := ticker.BidQty + ticker.AskQty
	pressure := 0.0
	if bookDepth > 0 {
		pressure = (ticker.BidQty - ticker.AskQty) / bookDepth
	}

	row.Volume = ticker.Volume
	row.Pressure = pressure
	row.Change = ticker.ChangePct / 100
	row.Price = ticker.Last
	row.Updated = ticker.Timestamp

	crossSection.version++
	crossSection.refreshAggregates()

	return nil
}

func (crossSection *CrossSection) ReturnCap() int {
	return max(crossSection.cfg.ReturnCap, 2)
}

func (crossSection *CrossSection) MinBarsRequired() int {
	return max(crossSection.cfg.MinBars, 2)
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

	return crossSection.tail(row.Returns, window)
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

		if row.Volume > 0 && row.Price > 0 {
			values = append(values, row.Volume*row.Price)
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

	return row.Pressure
}

func (crossSection *CrossSection) Symbols() []string {
	symbols := make([]string, len(crossSection.symbols))
	copy(symbols, crossSection.symbols)

	return symbols
}

func (crossSection *CrossSection) ensure(name string) *CrossSectionRow {
	if row := crossSection.rows[name]; row != nil {
		return row
	}

	row := &CrossSectionRow{
		Symbol:  name,
		Returns: []float64{},
	}
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

		if row.Change > 0 {
			positive++
		}

		if row.Volume > 0 {
			volumes = append(volumes, row.Volume)
		}

		absChanges[symbol] = math.Abs(row.Change)
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

	crossSection.cfg.ReturnCap = max(crossSection.ReturnCap(), longWindow)
	crossSection.cfg.MinBars = max(crossSection.MinBarsRequired(), max(crossSection.cfg.ReturnCap/4, 2))
	crossSection.cfg.BreadthCap = max(crossSection.breadthCap(), crossSection.cfg.ReturnCap)
}

func (crossSection *CrossSection) breadthCap() int {
	return max(crossSection.cfg.BreadthCap, crossSection.ReturnCap())
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
