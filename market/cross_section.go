package market

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/statutil"
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
	window := max(statutil.SampleBudgetFromCadence(cadence), 2)
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

func (crossSection *CrossSection) Observe(row *Symbol) error {
	if row == nil {
		return errnie.Error(fmt.Errorf("cross-section: nil row"))
	}
	if err := row.Validate(); err != nil {
		return errnie.Error(err)
	}

	state := crossSection.ensure(row.Name)
	priorUpdated := datura.Peek[int64](state, "updated")
	if priorUpdated > 0 {
		gap := row.Updated.Sub(time.Unix(0, priorUpdated)).Seconds()
		if gap > 0 {
			crossSection.push(&crossSection.updateGaps, gap, statutil.SampleBudgetFromCadence(statutil.Median(crossSection.updateGaps)))
			crossSection.refreshConfig()
		}
	}

	returns := datura.Peek[[]float64](state, "returns")
	lastPrice := datura.Peek[float64](state, "price")
	if lastPrice > 0 {
		ret := math.Log(row.Price / lastPrice)
		if ret != 0 || len(returns) == 0 {
			crossSection.push(&returns, ret, crossSection.ReturnCap())
		}
	}

	state.Poke(row.Name, "symbol").
		Poke(returns, "returns").
		Poke(row.Volume, "volume").
		Poke(row.Pressure, "pressure").
		Poke(row.Value, "change").
		Poke(row.Price, "price").
		Poke(row.Updated.UnixNano(), "updated")

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
	return statutil.Median(crossSection.updateGaps)
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

	median := statutil.Median(changes)
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
	cadence := statutil.Median(crossSection.updateGaps)
	if cadence > 0 {
		crossSection.cfg = CrossSectionConfigFromCadence(cadence)
	}
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
	median := statutil.Median(sorted)
	deviations := make([]float64, 0, len(sorted))
	for _, value := range sorted {
		deviations = append(deviations, math.Abs(value-median))
	}

	return median + statutil.Median(deviations)
}
