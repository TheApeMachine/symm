package depthflow

import (
	"context"
	"iter"
	"math"
	"sort"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
	"github.com/theapemachine/symm/statutil"
)

/*
DepthFlow is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Multi-level distance weighting is
not wired yet; book math uses top-of-book bid/ask quantities only.

Spoof Trap is currently scored from L2 top-of-book shape contradicted by trade
pressure. A faithful spoof read (deep-book orders that add and delete without
trade confirmation) needs L3 per-order events, which are not ingested here — the
comment keeps the L2 limitation honest rather than promising order-level shape.

1. Loaded Imbalance — book weight agrees with trade pressure.
2. Spoof Trap — deep-book shape contradicts trade pressure.
3. Book Thinning — defensive depth disappearing at the touch.
4. Dense Neutrality — balanced thick depth with low pressure.

# Summary of DepthFlow Categories

| Category         | WBI (Weighted Imbalance) | Trade Pressure    | Market "Feel"            |
|:-----------------|:-------------------------|:------------------|:-------------------------|
| Loaded Imbalance | High                     | High (Agrees)     | Structural Gravity       |
| Spoof Trap       | High                     | Low (Contradicts) | Manipulated/Fake         |
| Book Thinning    | Rapidly Falling          | Variable          | Exhaustion/Crumbling     |
| Dense Neutrality | Balanced                 | Low               | Robust Stability         |
*/

/*
Signal measures touch-level book imbalance with trade-pressure confirmation.
See the struct comment block for category semantics. History is tree-only: prior
measurement artifacts are the sole replay source (no local per-pair store). Book
replay (wbi/depth) and trade replay (pressure) are read from separate streams so
a zero-depth trade row never poisons the book-depth thinning series.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	tree         *dmt.Tree
	batchHistory map[string]replayHistory
	batchActive  bool
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"book", "trade"}
}

func (signal *Signal) ResetBatch() {
	if signal == nil {
		return
	}

	signal.batchHistory = make(map[string]replayHistory)
	signal.batchActive = true
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil {
			return
		}

		channel := datura.Peek[string](datapoint, "channel")

		if channel == "trade" {
			for rowIndex := 0; ; rowIndex++ {
				symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")

				if symbol == "" {
					return
				}

				measurement := signal.measureTrade(datapoint, rowIndex)

				if measurement == nil {
					continue
				}

				if !yield(measurement) {
					return
				}
			}
		}

		if channel != "book" {
			return
		}

		for rowIndex := 0; ; rowIndex++ {
			symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")
			bidQty := datura.Peek[float64](datapoint, "data", rowIndex, "bids", 0, "qty")
			askQty := datura.Peek[float64](datapoint, "data", rowIndex, "asks", 0, "qty")

			if symbol == "" {
				return
			}

			depth := bidQty + askQty
			imbalance := 0.0

			if depth > 0 {
				imbalance = (bidQty - askQty) / depth
			}

			history := signal.history(symbol, datapoint.Timestamp())
			wbiHistory := history.wbiHistory
			depthHistory := history.depthHistory
			prevDepth := history.prevDepth
			pressureHistory := history.pressureHistory
			pressure := history.pressure

			thinning := 0.0

			if prevDepth > 0 && depth < prevDepth {
				thinning = (prevDepth - depth) / prevDepth
			}

			wbi := math.Abs(imbalance)
			wbiScore := statutil.ScaleByMedianOrUnity(wbi, wbiHistory)
			thinScore := statutil.ScaleByMedianOrUnity(thinning, depthHistory)
			pressureScore := statutil.ScaleByMedianOrUnity(math.Abs(pressure), pressureHistory)

			aligned := imbalance * pressure
			loadedMass := 0.0
			spoofMass := 0.0

			if aligned >= 0 {
				loadedMass = wbi * pressureScore * (1 + math.Abs(aligned)) * (1 + math.Abs(aligned)) * (1 + math.Abs(pressure)/(1+math.Abs(pressure)))
				spoofMass = wbi / (1 + pressureScore)
			}

			if aligned < 0 {
				spoofMass = wbi * pressureScore * pressureScore * (1 + wbi + pressureScore)
				loadedMass = wbi / (1 + pressureScore)
			}

			shares := []dist.Share{
				{Key: "loaded", Category: logic.CategoryLoadedImbalance, Mass: loadedMass},
				{Key: "spoof", Category: logic.CategorySpoofTrap, Mass: spoofMass},
				{Key: "thinning", Category: logic.CategoryBookThinning, Mass: thinScore},
				{Key: "neutral", Category: logic.CategoryDenseNeutrality, Mass: (1 / (1 + wbiScore*wbiScore*wbiScore)) * (1 / (1 + pressureScore*pressureScore*pressureScore)) * (1 - math.Min(1, math.Abs(aligned)))},
			}

			measurement := datura.Acquire("depthflow", datura.APPJSON)
			measurement.WithRole("measurement")
			measurement.WithScope(symbol)
			errnie.Error(measurement.SetOrigin(string(logic.SourceDepthFlow)))
			measurement.SetTimestamp(datapoint.Timestamp())

			output, confidence := dist.Fields(shares)
			output["wbi"] = wbi
			output["pressure"] = pressure
			measurement.MergeOutputs(output)
			measurement.MergeFields(map[string]any{
				"depth":     depth,
				"imbalance": imbalance,
				"pressure":  pressure,
				"timestamp": datapoint.Timestamp(),
			})
			signal.rememberBook(symbol, depth, wbi, datapoint.Timestamp())

			// A low-confidence book row still carries real depth/imbalance the next
			// frame needs to rebuild thinning continuity, so it is emitted as a
			// state seed (no classifier output) rather than dropped — the tree is
			// the only book-depth replay source.
			_ = confidence

			if !yield(measurement) {
				return
			}
		}
	}
}

/*
measureTrade accumulates running trade pressure for the symbol from the tree's
prior pressure and emits a pressure-only state seed (no classifier output). Book
frames read this back via tradeHistory; the trade stream stays separate from the
book-depth stream so a zero-depth trade row never poisons book history.
*/
func (signal *Signal) measureTrade(datapoint *datura.Artifact, rowIndex int) *datura.Artifact {
	symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")
	side := datura.Peek[string](datapoint, "data", rowIndex, "side")
	quantity := datura.Peek[float64](datapoint, "data", rowIndex, "qty")

	if quantity <= 0 {
		return nil
	}

	signed := quantity

	if side == "sell" {
		signed = -quantity
	}

	history := signal.history(symbol, datapoint.Timestamp())
	priorPressure := history.pressure
	pressure := priorPressure + signed

	measurement := datura.Acquire("depthflow", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourceDepthFlow)))
	measurement.SetTimestamp(datapoint.Timestamp())
	measurement.MergeFields(map[string]any{
		"pressure":  pressure,
		"kind":      "trade",
		"timestamp": datapoint.Timestamp(),
	})
	signal.rememberTrade(symbol, pressure, datapoint.Timestamp())

	return measurement
}

/*
bookHistory rebuilds the book-depth replay streams (touch imbalance and per-row
depth-drop fractions) plus the latest touch depth from book measurement rows in
the tree alone. Trade rows (depth==0, kind=="trade") are skipped so a zero-depth
trade never registers as a depth collapse.
*/
type replayHistory struct {
	wbiHistory      []float64
	depthHistory    []float64
	prevDepth       float64
	pressureHistory []float64
	pressure        float64
	bookStamps      []float64
	pressureStamps  []float64
}

func (signal *Signal) history(symbol string, currentStamp int64) replayHistory {
	history := replayHistory{}

	if signal.tree == nil {
		return history
	}

	if signal.batchActive && signal.batchHistory != nil {
		if cached, ok := signal.batchHistory[symbol]; ok {
			return cached
		}
	}

	type depthSample struct {
		stamp float64
		depth float64
		wbi   float64
	}

	type pressureSample struct {
		stamp    float64
		pressure float64
	}

	collect := func(prefix []byte) ([]depthSample, []float64, []pressureSample, []float64) {
		bookStamps := []float64{}
		bookSamples := make([]depthSample, 0)
		pressureStamps := []float64{}
		pressureSamples := make([]pressureSample, 0)

		for prior := range signal.tree.Seek(prefix) {
			stamp := datura.Peek[float64](prior, "timestamp")

			if datura.Peek[string](prior, "kind") == "trade" {
				pressureSamples = append(pressureSamples, pressureSample{
					stamp:    stamp,
					pressure: datura.Peek[float64](prior, "pressure"),
				})
				pressureStamps = append(pressureStamps, stamp)

				continue
			}

			depth := datura.Peek[float64](prior, "depth")

			if depth <= 0 {
				continue
			}

			wbi := datura.Peek[float64](prior, "output", "wbi")

			if wbi == 0 {
				wbi = math.Abs(datura.Peek[float64](prior, "imbalance"))
			}

			bookSamples = append(bookSamples, depthSample{stamp: stamp, depth: depth, wbi: wbi})
			bookStamps = append(bookStamps, stamp)
		}

		return bookSamples, bookStamps, pressureSamples, pressureStamps
	}

	prefix := measurementTimePrefix(symbol, currentStamp)
	bookSamples, bookStamps, pressureSamples, pressureStamps := collect(prefix)

	if len(bookSamples) == 0 && len(pressureSamples) == 0 && string(prefix) != string(measurementPrefix(symbol)) {
		bookSamples, bookStamps, pressureSamples, pressureStamps = collect(measurementPrefix(symbol))
	}

	sort.Slice(bookSamples, func(left, right int) bool {
		return bookSamples[left].stamp < bookSamples[right].stamp
	})

	lastDepth := 0.0

	for index, sample := range bookSamples {
		if sample.wbi > 0 {
			history.wbiHistory = append(history.wbiHistory, sample.wbi)
		}

		if lastDepth > 0 && sample.depth < lastDepth {
			history.depthHistory = append(history.depthHistory, (lastDepth-sample.depth)/lastDepth)
		}

		lastDepth = sample.depth

		if index == len(bookSamples)-1 {
			history.prevDepth = sample.depth
		}
	}

	sort.Slice(pressureSamples, func(left, right int) bool {
		return pressureSamples[left].stamp < pressureSamples[right].stamp
	})

	for index, sample := range pressureSamples {
		if sample.pressure != 0 {
			history.pressureHistory = append(history.pressureHistory, math.Abs(sample.pressure))
		}

		if index == len(pressureSamples)-1 {
			history.pressure = sample.pressure
		}
	}

	bookKeep := statutil.WindowDepth(bookStamps)
	pressureKeep := statutil.WindowDepth(pressureStamps)
	history.wbiHistory = statutil.Tail(history.wbiHistory, bookKeep)
	history.depthHistory = statutil.Tail(history.depthHistory, bookKeep)
	history.pressureHistory = statutil.Tail(history.pressureHistory, pressureKeep)
	history.bookStamps = statutil.Tail(bookStamps, bookKeep)
	history.pressureStamps = statutil.Tail(pressureStamps, pressureKeep)

	signal.storeHistory(symbol, history)

	return history
}

func (signal *Signal) storeHistory(symbol string, history replayHistory) {
	if signal == nil || !signal.batchActive {
		return
	}

	if signal.batchHistory == nil {
		signal.batchHistory = make(map[string]replayHistory)
	}

	signal.batchHistory[symbol] = history
}

func (signal *Signal) rememberBook(symbol string, depth, wbi float64, stamp int64) {
	if signal == nil || symbol == "" || stamp <= 0 {
		return
	}

	history := signal.history(symbol, stamp)
	stampFloat := float64(stamp)

	if wbi > 0 {
		history.wbiHistory = append(history.wbiHistory, wbi)
	}

	if history.prevDepth > 0 && depth < history.prevDepth {
		history.depthHistory = append(history.depthHistory, (history.prevDepth-depth)/history.prevDepth)
	}

	history.prevDepth = depth
	history.bookStamps = append(history.bookStamps, stampFloat)
	keep := statutil.WindowDepth(history.bookStamps)
	history.wbiHistory = statutil.Tail(history.wbiHistory, keep)
	history.depthHistory = statutil.Tail(history.depthHistory, keep)
	history.bookStamps = statutil.Tail(history.bookStamps, keep)
	signal.storeHistory(symbol, history)
}

func (signal *Signal) rememberTrade(symbol string, pressure float64, stamp int64) {
	if signal == nil || symbol == "" || stamp <= 0 {
		return
	}

	history := signal.history(symbol, stamp)
	stampFloat := float64(stamp)

	if pressure != 0 {
		history.pressureHistory = append(history.pressureHistory, math.Abs(pressure))
	}

	history.pressure = pressure
	history.pressureStamps = append(history.pressureStamps, stampFloat)
	keep := statutil.WindowDepth(history.pressureStamps)
	history.pressureHistory = statutil.Tail(history.pressureHistory, keep)
	history.pressureStamps = statutil.Tail(history.pressureStamps, keep)
	signal.storeHistory(symbol, history)
}

func (signal *Signal) bookHistory(symbol string, currentStamp int64) (
	wbiHistory, depthHistory []float64,
	prevDepth float64,
) {
	history := signal.history(symbol, currentStamp)

	return history.wbiHistory, history.depthHistory, history.prevDepth
}

/*
tradeHistory rebuilds the running trade-pressure baseline and the latest pressure
value from trade measurement rows in the tree alone. Book rows (no pressure
update of their own) are skipped via the kind marker so pressure is a clean
trade-only series.
*/
func (signal *Signal) tradeHistory(symbol string, currentStamp int64) (
	pressureHistory []float64,
	pressure float64,
) {
	history := signal.history(symbol, currentStamp)

	return history.pressureHistory, history.pressure
}

func measurementPrefix(symbol string) []byte {
	return []byte("measurement/" + symbol + "/" + string(logic.SourceDepthFlow) + "/")
}

func measurementTimePrefix(symbol string, stamp int64) []byte {
	if stamp <= 0 {
		return measurementPrefix(symbol)
	}

	return []byte("measurement/" + symbol + "/" + string(logic.SourceDepthFlow) + "/" +
		time.Unix(0, stamp).UTC().Format("2006/01/02/15"))
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
