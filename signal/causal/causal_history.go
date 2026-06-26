package causal

import (
	"math"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/statutil"
)

/*
historian reconstructs a symbol's causal context from the tree alone — the only
history store. It replays prior measurement artifacts for flow/return/spread
windows, seeks the live L2 book for spread and void evidence, and derives the
sector macro drift from cross-section peers. It owns no per-symbol cache; every
frame rebuilds state from the tree, so replicas and restarts stay consistent.
*/
type historian struct {
	tree *dmt.Tree
}

/*
NewHistorian returns a tree-backed causal historian.
*/
func NewHistorian(tree *dmt.Tree) historian {
	return historian{tree: tree}
}

type priorSample struct {
	stamp    float64
	flow     float64
	velocity float64
	price    float64
}

/*
window replays this symbol's prior causal measurements from the tree, returning
the cadence-derived flow and return tails plus the most recent price. The first
observation has no prior window — callers score it at low confidence rather than
gating on a sample count.
*/
func (causalHistorian historian) window(symbol string) (
	flowHistory, velocityHistory []float64,
	prevPrice float64,
) {
	if causalHistorian.tree == nil {
		return nil, nil, 0
	}

	samples := make([]priorSample, 0)

	for prior := range causalHistorian.tree.Seek(measurementPrefix(symbol)) {
		sample := priorSample{
			stamp:    datura.Peek[float64](prior, "timestamp"),
			flow:     math.Abs(datura.Peek[float64](prior, "flow")),
			velocity: math.Abs(datura.Peek[float64](prior, "velocity")),
			price:    datura.Peek[float64](prior, "price"),
		}

		if sample.price <= 0 || sample.stamp <= 0 {
			continue
		}

		samples = append(samples, sample)
	}

	if len(samples) == 0 {
		return nil, nil, 0
	}

	keep := windowKeep(samples)

	if keep <= 0 {
		return nil, nil, 0
	}

	samples = samples[len(samples)-keep:]
	flowHistory = make([]float64, len(samples))
	velocityHistory = make([]float64, len(samples))
	prevPrice = samples[len(samples)-1].price

	for index := range samples {
		flowHistory[index] = samples[index].flow
		velocityHistory[index] = samples[index].velocity
	}

	return flowHistory, velocityHistory, prevPrice
}

func windowKeep(samples []priorSample) int {
	stamps := make([]float64, len(samples))

	for index := range samples {
		stamps[index] = samples[index].stamp
	}

	keep := statutil.WindowDepth(stamps)

	if keep > len(samples) {
		return len(samples)
	}

	return keep
}

/*
bookStress derives liquidity stress for the symbol from the live L2 book — real
touch spread scaled against the symbol's recent spread baseline, amplified by
book void (collapsed or one-sided top-of-book depth). This replaces the prior
ticker-summary spread proxy with genuine microstructure: a fragile, voided book
reads as shock; a deep two-sided book does not.
*/
func (causalHistorian historian) bookStress(symbol string, currentStamp float64) (
	stress, rawSpread float64,
) {
	spread, void, ok := causalHistorian.book(symbol, currentStamp)

	if !ok {
		return 0, 0
	}

	baseline := causalHistorian.spreadBaseline(symbol, currentStamp)

	// A voided touch (empty or one-sided depth) is the liquidity-shock signature
	// the ticker summary cannot see: widen the stress by the void fraction.
	return statutil.ScaleByMedian(spread, baseline) * (1 + void), spread
}

func (causalHistorian historian) spreadBaseline(symbol string, currentStamp float64) []float64 {
	baseline := make([]float64, 0)
	stamps := make([]float64, 0)

	for prior := range causalHistorian.tree.Seek(measurementPrefix(symbol)) {
		stamp := datura.Peek[float64](prior, "timestamp")

		if stamp <= 0 || (currentStamp > 0 && stamp >= currentStamp) {
			continue
		}

		spread := datura.Peek[float64](prior, "spread")

		if spread <= 0 {
			spread = datura.Peek[float64](prior, "output", "spread")
		}

		if spread <= 0 {
			continue
		}

		baseline = append(baseline, spread)
		stamps = append(stamps, stamp)
	}

	keep := statutil.WindowDepth(stamps)

	return statutil.Tail(baseline, keep)
}

/*
book seeks the latest L2 book frame at or before currentStamp and returns the
touch spread and a void fraction (0 = deep two-sided book, 1 = fully collapsed
or one-sided touch).
*/
func (causalHistorian historian) book(symbol string, currentStamp float64) (
	spread, void float64, ok bool,
) {
	if causalHistorian.tree == nil {
		return 0, 0, false
	}

	latestStamp := 0.0

	for artifact := range causalHistorian.tree.Seek([]byte("book/")) {
		stamp := float64(artifact.Timestamp())

		if currentStamp > 0 && stamp > currentStamp {
			break
		}

		for rowIndex := 0; ; rowIndex++ {
			rowSymbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")

			if rowSymbol == "" {
				break
			}

			if rowSymbol != symbol {
				continue
			}

			rowSpread, rowVoid, rowOK := readBookRow(artifact, rowIndex)

			if !rowOK || stamp < latestStamp {
				continue
			}

			spread = rowSpread
			void = rowVoid
			latestStamp = stamp
			ok = true
		}
	}

	return spread, void, ok
}

func readBookRow(artifact *datura.Artifact, rowIndex int) (spread, void float64, ok bool) {
	bidPrice := datura.Peek[float64](artifact, "data", rowIndex, "bids", 0, "price")
	askPrice := datura.Peek[float64](artifact, "data", rowIndex, "asks", 0, "price")
	bidQty := datura.Peek[float64](artifact, "data", rowIndex, "bids", 0, "qty")
	askQty := datura.Peek[float64](artifact, "data", rowIndex, "asks", 0, "qty")

	// A missing or crossed touch is a fully voided book: maximal shock evidence.
	if bidPrice <= 0 || askPrice <= 0 || askPrice < bidPrice {
		return 0, 1, false
	}

	spread = askPrice - bidPrice

	// Void fraction: a one-sided touch (one quote empty) reads as half-collapsed;
	// a missing side reads as fully collapsed. Two-sided depth voids nothing.
	if bidQty <= 0 && askQty <= 0 {
		return spread, 1, true
	}

	if bidQty <= 0 || askQty <= 0 {
		return spread, 0.5, true
	}

	return spread, 0, true
}

/*
macroDrift derives the systemic sector momentum this symbol shares with its
peers from the cross-section return window. It is the Systemic Beta driver.
*/
func (causalHistorian historian) macroDrift(
	symbol string,
	crossSection *market.CrossSection,
) float64 {
	if crossSection == nil {
		return 0
	}

	window := crossSection.MaxReturnWindow()
	snapshot := crossSection.PeerWindowSnapshot(crossSection.MinBarsRequired(), time.Time{})
	returns := snapshot.MarketReturns

	if len(returns) == 0 {
		returns = crossSection.SymbolReturns(symbol, window)
	}

	if len(returns) == 0 {
		return 0
	}

	total := 0.0

	for _, value := range returns {
		total += math.Abs(value)
	}

	return total * math.Sqrt(float64(len(returns)))
}

func measurementPrefix(symbol string) []byte {
	return []byte("measurement/" + symbol + "/" + string(logic.SourceCausal) + "/")
}
