package types

import (
	"math"
	"strings"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/correlation"
	"github.com/theapemachine/symm/kraken"
)

const (
	defaultReturnCap  = 64
	defaultMinBars    = 4
	defaultBreadthCap = 128
)

type CrossSectionConfig struct {
	ReturnCap  int
	MinBars    int
	BreadthCap int
}

/*
DefaultCrossSectionConfig returns the bounds every production CrossSection
runs with. Tests that need different bounds build their own config; nothing
here is a per-symbol assumption.
*/
func DefaultCrossSectionConfig() CrossSectionConfig {
	return CrossSectionConfig{
		ReturnCap:  defaultReturnCap,
		MinBars:    defaultMinBars,
		BreadthCap: defaultBreadthCap,
	}
}

/*
SymbolMetrics is one symbol's row in a published CrossSectionSummary.
*/
type SymbolMetrics struct {
	Symbol          string  `json:"symbol"`
	Volume          float64 `json:"volume"`
	QuoteNotional   float64 `json:"quoteNotional"`
	ExecutableDepth float64 `json:"executableDepth"`
	LatestChange    float64 `json:"latestChange"`
}

type CrossSectionSummary struct {
	Metrics             []SymbolMetrics `json:"metrics"`
	Leader              string          `json:"leader"`
	LeadershipThreshold float64         `json:"leadershipThreshold"`
	Breadth             float64         `json:"breadth"`
}

/*
observation is what a ring slot actually holds. Kraken's Decimal.Float64
walks through math/big.Rat, which allocates; SymbolReturns and
SymbolSamples read the same historical slots repeatedly (once per peer,
every tick, for every subject), so that conversion is paid once here at
ingestion instead of on every one of those reads.
*/
type observation struct {
	at              time.Time
	last            float64
	changePct       float64
	volume          float64
	quoteNotional   float64
	executableDepth float64
}

/*
CrossSection tracks every symbol's recent ticker history in fixed-size
per-symbol ring buffers, so the cost of a tick is bounded by how many
symbols actually updated, not by any growing map or slice. Symbols are
registered lazily on first observation because the tracked universe is
discovered asynchronously (instrument discovery finishes after this
CrossSection already exists), not known upfront.

ProcessUpdates is the single writer; it must only ever be called
sequentially from the planner's tick loop. ReadView is safe for concurrent
readers because it hands out an immutable, atomically-published snapshot.
*/
type CrossSection struct {
	config    CrossSectionConfig
	symbols   []string
	symbolMap map[string]int

	// history[i] is symbol i's ring buffer; ringIdx[i] is its current head.
	history [][]observation
	ringIdx []int
	// counts[i] is how many bars symbol i has ever received, saturating at
	// ReturnCap. A symbol needs at least MinBars of these before its change
	// is trusted enough to count toward the leadership threshold.
	counts []int

	activeView atomic.Pointer[CrossSectionSummary]
}

func NewCrossSection(config CrossSectionConfig) (*CrossSection, error) {
	if config.ReturnCap <= 0 || config.MinBars <= 1 {
		return nil, errnie.Error(errnie.Err(errnie.Validation, "invalid bounds parameters", nil))
	}

	crossSection := &CrossSection{
		config:    config,
		symbolMap: make(map[string]int),
	}

	crossSection.activeView.Store(&CrossSectionSummary{})

	return crossSection, nil
}

/*
MaxReturnWindow reports how many bars of history each symbol's ring can
hold, so callers know how large a buffer to pass to SymbolReturns and
SymbolSamples.
*/
func (crossSection *CrossSection) MaxReturnWindow() int {
	return crossSection.config.ReturnCap
}

/*
ProcessUpdates folds raw ticker rows into the per-symbol ring buffers. A
symbol seen for the first time is registered on the spot; every symbol
seen before this reuses its existing ring with zero further allocation.
Must be called sequentially inside the planner's tick loop.
*/
func (crossSection *CrossSection) ProcessUpdates(rows []kraken.TickerData) {
	mutated := false
	capLen := crossSection.config.ReturnCap + 1

	for _, row := range rows {
		symbol := strings.TrimSpace(row.Symbol)

		if symbol == "" || row.Timestamp.IsZero() || row.Last == nil || row.Last.Sign() <= 0 {
			continue
		}

		index, exists := crossSection.symbolMap[symbol]

		if !exists {
			index = len(crossSection.symbols)
			crossSection.symbolMap[symbol] = index
			crossSection.symbols = append(crossSection.symbols, symbol)
			crossSection.history = append(crossSection.history, make([]observation, capLen))
			crossSection.ringIdx = append(crossSection.ringIdx, 0)
			crossSection.counts = append(crossSection.counts, 0)
		}

		currentPos := crossSection.ringIdx[index]
		latest := crossSection.history[index][currentPos]

		if !latest.at.IsZero() && !row.Timestamp.After(latest.at) {
			continue
		}

		nextPos := (currentPos + 1) % capLen
		crossSection.history[index][nextPos] = observation{
			at:              row.Timestamp,
			last:            row.Last.Float64(),
			changePct:       row.ChangePct,
			volume:          row.Volume,
			quoteNotional:   QuoteNotional(row),
			executableDepth: ExecutableDepth(row),
		}
		crossSection.ringIdx[index] = nextPos

		if crossSection.counts[index] < crossSection.config.ReturnCap {
			crossSection.counts[index]++
		}

		mutated = true
	}

	if mutated {
		crossSection.recomputeSummary()
	}
}

/*
ReadView hands out the most recently published summary. Lock-free and safe
for any number of concurrent readers; the returned pointer is never
mutated after publication.
*/
func (crossSection *CrossSection) ReadView() *CrossSectionSummary {
	return crossSection.activeView.Load()
}

func (crossSection *CrossSection) recomputeSummary() {
	metrics, changes, positive, total, leader, _ :=
		crossSection.accumulateSummaryMetrics()

	var breadth float64
	if total > 0 {
		breadth = positive / total
	}

	crossSection.activeView.Store(&CrossSectionSummary{
		Metrics:             metrics,
		Leader:              leader,
		LeadershipThreshold: leadershipThreshold(changes),
		Breadth:             breadth,
	})
}

/*
SymbolReturns writes symbol's log returns, oldest first, into dst and
reports how many were written. The lookback is len(dst) bars; dst is
caller-owned so a hot loop (e.g. correlation scoring every peer every
tick) can reuse one buffer instead of allocating per call.

Call this sequentially on the writer/tick-loop goroutine after
ProcessUpdates completes; it is not safe for concurrent callers.
*/
func (crossSection *CrossSection) SymbolReturns(symbol string, dst []float64) int {
	if len(dst) == 0 {
		return 0
	}

	window := crossSection.MaxReturnWindow()

	if len(dst) > window {
		dst = dst[:window]
	}

	index, exists := crossSection.symbolMap[strings.TrimSpace(symbol)]

	if !exists {
		return 0
	}

	capLen := crossSection.config.ReturnCap + 1
	head := crossSection.ringIdx[index]
	written := 0

	for offset := range dst {
		currentPos := (head - offset + capLen) % capLen
		previousPos := (head - offset - 1 + capLen) % capLen

		current := crossSection.history[index][currentPos]
		previous := crossSection.history[index][previousPos]

		if current.at.IsZero() || previous.at.IsZero() {
			break
		}

		if current.last <= 0 || previous.last <= 0 {
			continue
		}

		dst[written] = math.Log(current.last / previous.last)
		written++
	}

	reverseFloat64s(dst[:written])

	return written
}

/*
SymbolSamples writes symbol's recent (timestamp, price) samples, oldest
first, into dst and reports how many were written. dst is caller-owned for
the same reuse reason as SymbolReturns. Chronological order matters here:
callers correlate two symbols' samples pairwise over time (e.g.
Hayashi-Yoshida), which requires walking both series forward in time.

Call this sequentially on the writer/tick-loop goroutine after
ProcessUpdates completes; it is not safe for concurrent callers.
*/
func (crossSection *CrossSection) SymbolSamples(symbol string, dst []correlation.Sample) int {
	if len(dst) == 0 {
		return 0
	}

	window := crossSection.MaxReturnWindow()

	if len(dst) > window {
		dst = dst[:window]
	}

	index, exists := crossSection.symbolMap[strings.TrimSpace(symbol)]

	if !exists {
		return 0
	}

	capLen := crossSection.config.ReturnCap + 1
	head := crossSection.ringIdx[index]
	written := 0

	for offset := range dst {
		pos := (head - offset + capLen) % capLen
		entry := crossSection.history[index][pos]

		if entry.at.IsZero() {
			break
		}

		if entry.last <= 0 {
			continue
		}

		dst[written] = correlation.Sample{At: entry.at, Value: entry.last}
		written++
	}

	reverseSamples(dst[:written])

	return written
}

func reverseFloat64s(values []float64) {
	for left, right := 0, len(values)-1; left < right; left, right = left+1, right-1 {
		values[left], values[right] = values[right], values[left]
	}
}

func reverseSamples(samples []correlation.Sample) {
	for left, right := 0, len(samples)-1; left < right; left, right = left+1, right-1 {
		samples[left], samples[right] = samples[right], samples[left]
	}
}

func QuoteNotional(row kraken.TickerData) float64 {
	rate := row.Vwap

	if rate <= 0 {
		rate = row.Last.Float64()
	}

	if rate <= 0 || row.Volume <= 0 {
		return 0
	}

	return row.Volume * rate
}

func ExecutableDepth(row kraken.TickerData) float64 {
	if row.Bid == nil || row.Ask == nil {
		return 0
	}

	bid := row.Bid.Float64()
	ask := row.Ask.Float64()

	if bid <= 0 || ask <= 0 {
		return 0
	}

	quantity := min(row.BidQty, row.AskQty)

	if quantity <= 0 {
		return 0
	}

	return quantity * (bid + ask) / 2
}
