package causal

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/statutil"
)

/*
historian reconstructs a symbol's causal context from the tree alone — the only
history store. It cold-starts prior measurement artifacts for flow/return/spread
windows from the tree, then keeps a per-symbol read-through mirror for the hot
path. The cache is not authoritative: emitted measurements still carry replay
fields and the tree remains the restart/replica source.
*/
type historian struct {
	tree  *dmt.Tree
	cache sync.Map
}

/*
NewHistorian returns a tree-backed causal historian.
*/
func NewHistorian(tree *dmt.Tree) *historian {
	return &historian{tree: tree}
}

type priorSample struct {
	stamp    float64
	flow     float64
	velocity float64
	price    float64
}

type cachedPriorSamples struct {
	mu      sync.Mutex
	samples []priorSample
}

/*
window replays this symbol's prior causal measurements from the tree, returning
the cadence-derived flow and return tails plus the most recent price. The first
observation has no prior window — callers score it at low confidence rather than
gating on a sample count.
*/
func (causalHistorian *historian) window(symbol string, currentStamp int64) (
	flowHistory, velocityHistory []float64,
	prevPrice float64,
) {
	if causalHistorian == nil || causalHistorian.tree == nil {
		return nil, nil, 0
	}

	windowStart := currentStamp - int64(12*time.Hour)
	if samples := causalHistorian.cachedWindow(symbol, windowStart, currentStamp); len(samples) > 0 {
		return samplesToWindow(samples)
	}

	samples := causalHistorian.loadWindowFromTree(symbol, windowStart, currentStamp)
	if len(samples) == 0 {
		return nil, nil, 0
	}

	causalHistorian.storeSamples(symbol, samples)

	return samplesToWindow(samples)
}

func (causalHistorian *historian) loadWindowFromTree(
	symbol string,
	windowStart int64,
	currentStamp int64,
) []priorSample {
	samples := make([]priorSample, 0)
	rolePrefix := "measurement/" + symbol + "/" + string(logic.SourceCausal)
	for _, seekKey := range dailyPrefixes(rolePrefix, "", windowStart, currentStamp) {
		for prior := range causalHistorian.tree.Seek(seekKey) {
			stamp := datura.Peek[float64](prior, "timestamp")
			if stamp == 0 {
				stamp = float64(prior.Timestamp())
			}

			if int64(stamp) < windowStart {
				continue
			}

			if int64(stamp) > currentStamp {
				break
			}

			sample := priorSample{
				stamp:    stamp,
				flow:     math.Abs(datura.Peek[float64](prior, "flow")),
				velocity: math.Abs(datura.Peek[float64](prior, "velocity")),
				price:    datura.Peek[float64](prior, "price"),
			}

			if sample.price <= 0 {
				continue
			}

			samples = append(samples, sample)
		}
	}

	sort.Slice(samples, func(left, right int) bool {
		return samples[left].stamp < samples[right].stamp
	})

	return samples
}

func samplesToWindow(samples []priorSample) (
	flowHistory, velocityHistory []float64,
	prevPrice float64,
) {
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

func (causalHistorian *historian) cachedWindow(
	symbol string,
	windowStart int64,
	currentStamp int64,
) []priorSample {
	value, ok := causalHistorian.cache.Load(symbol)
	if !ok {
		return nil
	}

	cached, ok := value.(*cachedPriorSamples)
	if !ok || cached == nil {
		return nil
	}

	cached.mu.Lock()
	defer cached.mu.Unlock()

	samples := make([]priorSample, 0, len(cached.samples))
	for _, sample := range cached.samples {
		if int64(sample.stamp) < windowStart || int64(sample.stamp) > currentStamp {
			continue
		}
		samples = append(samples, sample)
	}

	sort.Slice(samples, func(left, right int) bool {
		return samples[left].stamp < samples[right].stamp
	})

	return samples
}

func (causalHistorian *historian) remember(
	symbol string,
	stamp int64,
	flow float64,
	velocity float64,
	price float64,
) {
	if causalHistorian == nil || symbol == "" || stamp <= 0 || price <= 0 {
		return
	}

	value, _ := causalHistorian.cache.LoadOrStore(symbol, &cachedPriorSamples{})
	cached, ok := value.(*cachedPriorSamples)
	if !ok || cached == nil {
		return
	}

	cached.mu.Lock()
	defer cached.mu.Unlock()

	stampFloat := float64(stamp)
	for index, sample := range cached.samples {
		if sample.stamp != stampFloat {
			continue
		}

		cached.samples[index] = priorSample{
			stamp:    stampFloat,
			flow:     math.Abs(flow),
			velocity: math.Abs(velocity),
			price:    price,
		}
		return
	}

	cached.samples = append(cached.samples, priorSample{
		stamp:    stampFloat,
		flow:     math.Abs(flow),
		velocity: math.Abs(velocity),
		price:    price,
	})

	windowStart := stamp - int64(12*time.Hour)
	write := 0
	for _, sample := range cached.samples {
		if int64(sample.stamp) >= windowStart {
			cached.samples[write] = sample
			write++
		}
	}
	cached.samples = cached.samples[:write]
}

func (causalHistorian *historian) storeSamples(symbol string, samples []priorSample) {
	if causalHistorian == nil || symbol == "" {
		return
	}

	value, _ := causalHistorian.cache.LoadOrStore(symbol, &cachedPriorSamples{})
	cached, ok := value.(*cachedPriorSamples)
	if !ok || cached == nil {
		return
	}

	cached.mu.Lock()
	defer cached.mu.Unlock()

	cached.samples = append(cached.samples[:0], samples...)
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
func (causalHistorian *historian) bookStress(symbol string, currentStamp float64) (
	stress, rawSpread float64,
) {
	if causalHistorian == nil {
		return 0, 0
	}

	spread, void, ok := causalHistorian.book(symbol, currentStamp)

	if !ok {
		return 0, 0
	}

	baseline := causalHistorian.spreadBaseline(symbol, currentStamp)

	// A voided touch (empty or one-sided depth) is the liquidity-shock signature
	// the ticker summary cannot see: widen the stress by the void fraction.
	return statutil.ScaleByMedian(spread, baseline) * (1 + void), spread
}

func (causalHistorian *historian) spreadBaseline(symbol string, currentStamp float64) []float64 {
	if causalHistorian == nil || causalHistorian.tree == nil {
		return nil
	}

	baseline := make([]float64, 0)
	stamps := make([]float64, 0)
	currentStampNano := int64(currentStamp)
	windowStart := currentStampNano - int64(12*time.Hour)
	rolePrefix := "measurement/" + symbol + "/" + string(logic.SourceCausal)

	for _, seekKey := range dailyPrefixes(rolePrefix, "", windowStart, currentStampNano) {
		for prior := range causalHistorian.tree.Seek(seekKey) {
			stamp := datura.Peek[float64](prior, "timestamp")
			if stamp == 0 {
				stamp = float64(prior.Timestamp())
			}

			if int64(stamp) < windowStart {
				continue
			}

			if currentStamp > 0 && stamp >= currentStamp {
				break
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
	}

	keep := statutil.WindowDepth(stamps)

	return statutil.Tail(baseline, keep)
}

/*
book seeks the latest L2 book frame at or before currentStamp and returns the
touch spread and a void fraction (0 = deep two-sided book, 1 = fully collapsed
or one-sided touch).
*/
func (causalHistorian *historian) book(symbol string, currentStamp float64) (
	spread, void float64, ok bool,
) {
	if causalHistorian == nil || causalHistorian.tree == nil {
		return 0, 0, false
	}

	if spread, void, ok = causalHistorian.latestBook(symbol, currentStamp); ok {
		return spread, void, true
	}

	latestStamp := 0.0
	currentStampNano := int64(currentStamp)
	windowStart := currentStampNano - int64(12*time.Hour)

	for _, seekKey := range dailyPrefixes("book", symbol, windowStart, currentStampNano) {
		for artifact := range causalHistorian.tree.Seek(seekKey) {
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
	}

	return spread, void, ok
}

func (causalHistorian *historian) latestBook(symbol string, currentStamp float64) (
	spread, void float64, ok bool,
) {
	if causalHistorian == nil || causalHistorian.tree == nil {
		return 0, 0, false
	}

	raw, found := causalHistorian.tree.Get(latestScopedKey("book", symbol))

	if !found || len(raw) == 0 {
		return 0, 0, false
	}

	artifact := &datura.Artifact{}

	if _, err := artifact.Unpack(raw); err != nil {
		return 0, 0, false
	}

	if currentStamp > 0 && float64(artifact.Timestamp()) > currentStamp {
		return 0, 0, false
	}

	for rowIndex := 0; ; rowIndex++ {
		rowSymbol := datura.Peek[string](artifact, "data", rowIndex, "symbol")

		if rowSymbol == "" {
			return 0, 0, false
		}

		if rowSymbol != symbol {
			continue
		}

		return readBookRow(artifact, rowIndex)
	}
}

func latestScopedKey(role, symbol string) []byte {
	return []byte("latest/" + role + "/" + symbol)
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
func (causalHistorian *historian) macroDrift(
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

func dailyPrefixes(role string, symbol string, startNano, endNano int64) [][]byte {
	start := time.Unix(0, startNano).UTC().Truncate(24 * time.Hour)
	end := time.Unix(0, endNano).UTC().Truncate(24 * time.Hour)

	if end.Before(start) {
		end = start
	}

	prefixes := make([][]byte, 0, int(end.Sub(start)/(24*time.Hour))+1)

	for cursor := start; !cursor.After(end); cursor = cursor.AddDate(0, 0, 1) {
		dayStr := cursor.Format("2006/01/02")
		if symbol == "" {
			prefixes = append(prefixes, []byte(role+"/"+dayStr+"/"))
		} else {
			prefixes = append(prefixes, []byte(role+"/"+symbol+"/"+dayStr+"/"))
			prefixes = append(prefixes, []byte(role+"/"+symbol+"/kraken/"+dayStr+"/"))
		}
	}

	return prefixes
}
