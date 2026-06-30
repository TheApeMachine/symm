package market

import (
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"gonum.org/v1/gonum/stat"

	"github.com/theapemachine/symm/statutil"
)

/*
CrossSectionConfig sizes peer analytics buffers.
*/
type CrossSectionConfig struct {
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
	peerSeries       []peerReturnSeries
}

type peerReturnSeries struct {
	name    string
	returns []float64
}

/*
CrossSectionConfigFromCadence sizes peer analytics from observed universe cadence.
*/
func CrossSectionConfigFromCadence(cadence float64) *CrossSectionConfig {
	window := max(statutil.SampleBudgetFromCadence(cadence), 2)
	minBars := max(window/8, 2)
	returnCap := max(window/4, 2)

	return &CrossSectionConfig{
		ReturnCap:   returnCap,
		MinBars:     minBars,
		BreadthHist: returnCap,
	}
}

/*
DefaultCrossSectionConfig returns bootstrap sizing before universe cadence exists.
*/
func DefaultCrossSectionConfig() *CrossSectionConfig {
	return CrossSectionConfigFromCadence(0)
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
	mu               sync.RWMutex
	cfg              CrossSectionConfig
	universe         sync.Map
	symbols          []string
	breadths         []float64
	updateGaps       []float64
	cachedWindow     int
	cachedSnapshot   PeerSnapshot
	cacheValid       bool
	positiveChanges  int
	observedSymbols  int
	cachedVolumes    []float64
	cachedAbsChanges map[string]float64
}

/*
NewCrossSection allocates a cross-section store.
*/
func NewCrossSection(cfg *CrossSectionConfig) (*CrossSection, error) {
	if cfg == nil {
		return nil, errnie.Error(fmt.Errorf("cross-section: nil config"))
	}

	if cfg.ReturnCap < 2 {
		return nil, errnie.Error(fmt.Errorf("cross-section: return cap too small"))
	}

	if cfg.MinBars < 2 {
		return nil, errnie.Error(fmt.Errorf("cross-section: min bars too small"))
	}

	return &CrossSection{cfg: *cfg}, nil
}

func (crossSection *CrossSection) ensure(name string) *symbolState {
	raw, loaded := crossSection.universe.LoadOrStore(name, &symbolState{})

	state, ok := raw.(*symbolState)

	if !ok {
		return nil
	}

	if !loaded {
		crossSection.symbols = append(crossSection.symbols, name)
	}

	return state
}

func (crossSection *CrossSection) invalidatePeerCache() {
	crossSection.cacheValid = false
}

func (crossSection *CrossSection) refreshAggregates() {
	positive := 0
	total := 0
	volumes := make([]float64, 0, len(crossSection.symbols))
	absChanges := make(map[string]float64, len(crossSection.symbols))

	for _, name := range crossSection.symbols {
		raw, ok := crossSection.universe.Load(name)

		if !ok {
			continue
		}

		state, ok := raw.(*symbolState)

		if !ok {
			continue
		}

		total++

		if state.lastChange > 0 {
			positive++
		}

		if state.volume > 0 {
			volumes = append(volumes, state.volume)
		}

		absChanges[name] = math.Abs(state.lastChange)
	}

	crossSection.positiveChanges = positive
	crossSection.observedSymbols = total
	crossSection.cachedVolumes = volumes
	crossSection.cachedAbsChanges = absChanges
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

	crossSection.mu.Lock()
	defer crossSection.mu.Unlock()

	state := crossSection.ensure(row.Name)

	if state == nil {
		return errnie.Error(fmt.Errorf("cross-section: invalid state"))
	}

	if !state.updated.IsZero() {
		gap := row.Updated.Sub(state.updated).Seconds()

		if gap > 0 {
			gapCap := statutil.SampleBudgetFromCadence(statutil.Median(crossSection.updateGaps))
			pushCapped(&crossSection.updateGaps, gap, gapCap)
			crossSection.refreshConfig()
		}
	}

	if state.lastPrice <= 0 {
		state.lastPrice = row.Price
	} else {
		ret := math.Log(row.Price / state.lastPrice)

		if ret != 0 || len(state.returns) == 0 {
			pushCapped(&state.returns, ret, crossSection.cfg.ReturnCap)
		}
	}

	state.volume = row.Volume
	state.pressure = row.Pressure
	state.lastChange = row.Value
	state.lastPrice = row.Price
	state.updated = row.Updated

	crossSection.invalidatePeerCache()
	crossSection.refreshAggregates()

	return nil
}

/*
MinBarsRequired returns the minimum return window for peer scoring.
*/
func (crossSection *CrossSection) MinBarsRequired() int {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	return crossSection.cfg.MinBars
}

/*
MaxReturnWindow returns the longest return history available for drift scoring.
*/
func (crossSection *CrossSection) MaxReturnWindow() int {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	return crossSection.cfg.ReturnCap
}

/*
SymbolReturns returns the trailing return window for one symbol.
*/
func (crossSection *CrossSection) SymbolReturns(name string, window int) []float64 {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	return crossSection.symbolReturnsLocked(name, window)
}

func (crossSection *CrossSection) symbolReturnsLocked(name string, window int) []float64 {
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
/*
WarmPeers precomputes and caches the peer snapshot for window. It is the only
writer of the peer cache and must be called single-threaded (the trader's Observe
pass) before signals read concurrently. Signals must never trigger a cache write.
*/
func (crossSection *CrossSection) WarmPeers(window int) {
	crossSection.mu.Lock()
	defer crossSection.mu.Unlock()

	crossSection.cachedWindow = window
	crossSection.cachedSnapshot = crossSection.computePeerSnapshotLocked(window)
	crossSection.cacheValid = true
}

/*
PeerWindowSnapshot returns the peer snapshot for window. It is read-only: a cache
hit returns the warmed snapshot, a miss computes a fresh one without writing, so
many signals can call it concurrently during Measure without racing.
*/
func (crossSection *CrossSection) PeerWindowSnapshot(
	window int,
	_ time.Time,
) PeerSnapshot {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	return crossSection.peerWindowSnapshotLocked(window)
}

func (crossSection *CrossSection) peerWindowSnapshotLocked(window int) PeerSnapshot {
	if crossSection.cacheValid && crossSection.cachedWindow == window {
		return crossSection.cachedSnapshot
	}

	return crossSection.computePeerSnapshotLocked(window)
}

func (crossSection *CrossSection) computePeerSnapshotLocked(window int) PeerSnapshot {
	series := crossSection.peerSeriesLocked(window)

	if len(series) < 2 {
		return PeerSnapshot{}
	}

	return buildPeerSnapshot(series)
}

func (crossSection *CrossSection) peerSeriesLocked(window int) []peerReturnSeries {
	series := make([]peerReturnSeries, 0, len(crossSection.symbols))

	for _, name := range crossSection.symbols {
		raw, ok := crossSection.universe.Load(name)

		if !ok {
			continue
		}

		state, ok := raw.(*symbolState)

		if !ok {
			continue
		}

		returns := tailCopy(state.returns, window)

		if len(returns) < 1 {
			continue
		}

		effectiveWindow := len(returns)

		if effectiveWindow > window {
			effectiveWindow = window
		}

		series = append(series, peerReturnSeries{
			name:    name,
			returns: returns[len(returns)-effectiveWindow:],
		})
	}

	if len(series) < 1 {
		return nil
	}

	commonWindow := len(series[0].returns)

	for _, peer := range series[1:] {
		if len(peer.returns) < commonWindow {
			commonWindow = len(peer.returns)
		}
	}

	if commonWindow < 1 {
		return nil
	}

	for index := range series {
		series[index].returns = series[index].returns[len(series[index].returns)-commonWindow:]
	}

	return series
}

func buildPeerSnapshot(series []peerReturnSeries) PeerSnapshot {
	if len(series) < 2 {
		return PeerSnapshot{}
	}

	commonWindow := len(series[0].returns)
	marketReturns := make([]float64, commonWindow)

	for index := range commonWindow {
		column := make([]float64, len(series))

		for peerIndex, peer := range series {
			column[peerIndex] = peer.returns[index]
		}

		sort.Float64s(column)
		marketReturns[index] = stat.Quantile(0.5, stat.LinInterp, column, nil)
	}

	peerCorrelations := make([]float64, 0, len(series))
	peerEnergies := make([]float64, len(series))

	for _, peer := range series {
		peerCorrelation := stat.Correlation(peer.returns, marketReturns, nil)

		if !math.IsNaN(peerCorrelation) && !math.IsInf(peerCorrelation, 0) {
			peerCorrelations = append(peerCorrelations, peerCorrelation)
		}

		peerEnergies = append(peerEnergies, medianAbsolute(peer.returns))
	}

	return PeerSnapshot{
		MarketReturns:    marketReturns,
		PeerCorrelations: peerCorrelations,
		PeerEnergies:     peerEnergies,
		peerSeries:       series,
	}
}

func (snapshot PeerSnapshot) medianReturnsExcluding(name string, window int) []float64 {
	peers := make([]peerReturnSeries, 0, len(snapshot.peerSeries))

	for _, peer := range snapshot.peerSeries {
		if peer.name == name {
			continue
		}

		peers = append(peers, peer)
	}

	if len(peers) == 0 {
		return nil
	}

	marketReturns := make([]float64, window)

	for index := range window {
		column := make([]float64, len(peers))

		for peerIndex, peer := range peers {
			column[peerIndex] = peer.returns[index]
		}

		sort.Float64s(column)
		marketReturns[index] = stat.Quantile(0.5, stat.LinInterp, column, nil)
	}

	return marketReturns
}

/*
SymbolPeerStats returns this symbol's correlation with the cross-section median
return stream, its return energy, and the peer correlation slice used for gates.
The window shrinks to the available return depth when the universe is still warming.
*/
func (crossSection *CrossSection) SymbolPeerStats(name string, window int) (
	correlation float64,
	energy float64,
	peerCorrelations []float64,
	peerEnergyMedian float64,
	ok bool,
) {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	returns := crossSection.symbolReturnsLocked(name, window)

	if len(returns) < 2 {
		return 0, 0, nil, 0, false
	}

	effectiveWindow := len(returns)

	if effectiveWindow > window {
		effectiveWindow = window
	}

	snapshot := crossSection.peerWindowSnapshotLocked(effectiveWindow)

	commonWindow := len(snapshot.MarketReturns)

	if commonWindow < 2 {
		return 0, 0, nil, 0, false
	}

	returns = returns[len(returns)-commonWindow:]
	marketReturns := snapshot.medianReturnsExcluding(name, commonWindow)

	if len(marketReturns) < 2 {
		return 0, 0, nil, 0, false
	}

	correlation = stat.Correlation(returns, marketReturns, nil)

	if math.IsNaN(correlation) || math.IsInf(correlation, 0) {
		return 0, 0, nil, 0, false
	}

	return correlation, medianAbsolute(returns), snapshot.PeerCorrelations, medianPeerEnergy(snapshot.PeerEnergies), true
}

func medianPeerEnergy(energies []float64) float64 {
	if len(energies) == 0 {
		return 0
	}

	sorted := append([]float64(nil), energies...)
	sort.Float64s(sorted)

	return stat.Quantile(0.5, stat.LinInterp, sorted, nil)
}

/*
Breadth returns the fraction of symbols with positive last change at or before at.
*/
func (crossSection *CrossSection) Breadth(at time.Time) float64 {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	return crossSection.breadthLocked(at)
}

func (crossSection *CrossSection) breadthLocked(at time.Time) float64 {
	if at.IsZero() {
		if crossSection.observedSymbols == 0 {
			return 0
		}

		return float64(crossSection.positiveChanges) / float64(crossSection.observedSymbols)
	}

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
	crossSection.mu.Lock()
	defer crossSection.mu.Unlock()

	pushCapped(&crossSection.breadths, breadth, crossSection.cfg.BreadthHist)
}

/*
BreadthCount returns how many breadth samples have been recorded.
*/
func (crossSection *CrossSection) BreadthCount() int {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	return len(crossSection.breadths)
}

/*
MajorityThreshold returns the rolling median breadth.
*/
func (crossSection *CrossSection) MajorityThreshold(at time.Time) float64 {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	if len(crossSection.breadths) == 0 {
		return crossSection.breadthLocked(at)
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

	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	if at.IsZero() && len(crossSection.cachedAbsChanges) > 0 {
		return crossSection.isLeaderFromCache(name, change)
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

	threshold := leadershipThreshold(changes)

	return math.Abs(change) >= threshold
}

/*
LeadershipThreshold returns the current median+MAD absolute-change threshold
used for live leader selection at or before at.
*/
func (crossSection *CrossSection) LeadershipThreshold(at time.Time) float64 {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	changes := make([]float64, 0)

	if at.IsZero() && len(crossSection.cachedAbsChanges) > 0 {
		for _, absChange := range crossSection.cachedAbsChanges {
			changes = append(changes, absChange)
		}

		return leadershipThreshold(changes)
	}

	crossSection.universe.Range(func(_, value any) bool {
		state, ok := value.(*symbolState)

		if !ok {
			return true
		}

		if !state.updated.IsZero() && state.updated.After(at) {
			return true
		}

		changes = append(changes, math.Abs(state.lastChange))

		return true
	})

	return leadershipThreshold(changes)
}

/*
Volumes returns the latest quote volume per symbol.
*/
func (crossSection *CrossSection) Volumes() []float64 {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	if len(crossSection.cachedVolumes) == 0 {
		return nil
	}

	return append([]float64(nil), crossSection.cachedVolumes...)
}

/*
DollarVolumes returns latest price × volume per symbol for peer ranks that need
notional participation rather than base-unit volume.
*/
func (crossSection *CrossSection) DollarVolumes() []float64 {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	values := make([]float64, 0, len(crossSection.symbols))

	for _, name := range crossSection.symbols {
		raw, ok := crossSection.universe.Load(name)
		if !ok {
			continue
		}

		state, ok := raw.(*symbolState)
		if !ok || state.volume <= 0 || state.lastPrice <= 0 {
			continue
		}

		values = append(values, state.volume*state.lastPrice)
	}

	return values
}

func (crossSection *CrossSection) isLeaderFromCache(name string, change float64) bool {
	changes := make([]float64, 0, len(crossSection.cachedAbsChanges))

	for symbol, absChange := range crossSection.cachedAbsChanges {
		if symbol == name {
			changes = append(changes, math.Abs(change))

			continue
		}

		changes = append(changes, absChange)
	}

	if len(changes) < 2 {
		return false
	}

	threshold := leadershipThreshold(changes)

	return math.Abs(change) >= threshold
}

/*
Leader returns the symbol with the largest absolute change that also clears the
median+MAD leadership threshold — the pair the rest of the universe is chasing
right now. It is the dynamically derived lead-lag anchor: no config, no fixed
major, rotating as leadership rotates. Returns "" when no symbol leads.
*/
func (crossSection *CrossSection) Leader() string {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	if len(crossSection.cachedAbsChanges) < 2 {
		return ""
	}

	changes := make([]float64, 0, len(crossSection.cachedAbsChanges))
	leader := ""
	best := 0.0

	for name, absChange := range crossSection.cachedAbsChanges {
		changes = append(changes, absChange)

		if absChange > best {
			leader = name
			best = absChange
		}
	}

	// A flat universe has zero dispersion (MAD == 0): nobody stands apart, so
	// there is no leader to anchor on. The leader must strictly clear the
	// median+MAD band, not merely tie it.
	median := statutil.Median(changes)
	threshold := leadershipThreshold(changes)
	spread := threshold - median

	if spread <= 0 || best <= threshold {
		return ""
	}

	return leader
}

func leadershipThreshold(changes []float64) float64 {
	if len(changes) < 2 {
		return 0
	}

	sort.Float64s(changes)
	median := statutil.Median(changes)
	deviations := make([]float64, 0, len(changes))

	for _, value := range changes {
		deviations = append(deviations, math.Abs(value-median))
	}

	return median + statutil.Median(deviations)
}

/*
Pressure returns the latest pressure observation for one symbol.
*/
func (crossSection *CrossSection) Pressure(name string) float64 {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

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

func (crossSection *CrossSection) refreshConfig() {
	cadence := statutil.Median(crossSection.updateGaps)

	if cadence <= 0 {
		return
	}

	crossSection.cfg = *CrossSectionConfigFromCadence(cadence)
}

func pushCapped(buffer *[]float64, value float64, capacity int) {
	*buffer = append(*buffer, value)

	if len(*buffer) > capacity {
		*buffer = (*buffer)[len(*buffer)-capacity:]
	}
}

/*
Symbols returns a copy of the symbols tracked in the cross section.
*/
func (crossSection *CrossSection) Symbols() []string {
	crossSection.mu.RLock()
	defer crossSection.mu.RUnlock()

	symbolsCopy := make([]string, len(crossSection.symbols))
	copy(symbolsCopy, crossSection.symbols)

	return symbolsCopy
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
