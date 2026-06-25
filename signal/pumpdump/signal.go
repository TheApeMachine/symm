package pumpdump

import (
	"context"
	"iter"
	"math"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
	"github.com/theapemachine/symm/signal/dist"
	"github.com/theapemachine/symm/statutil"
)

/*
PumpDump is the Ignition perspective, identifying pre-pump microstructure by
looking for sudden "verticality" in volume and price.

1. What it measures exactly (in isolation)

Volume Lift (RVOL): Measures positive volume delta spikes against a
median-scaled baseline whose depth is derived from the pair's tick cadence.

Precursor Move: Scores upward price detachment from its recent anchor
(positive-only log return, scaled by its own recent median).

Spread Compression: Scores how much the bid/ask spread has tightened versus
its own median-scaled baseline.

Ignition Classifier: Maps rvol, precursor, compression, and rvol-decline into
four ignition states (not a symmetric pump/dump direction classifier).

---

2. Semantically, what story does it tell?

The PumpDump signal tells the story of explosive ignition and coiled energy.

The "Ignition" Story: It identifies the exact moment a move stops being random
walk and becomes a vertical event driven by abnormal volume "lift".

The "Coiled Spring" Story: By tracking spread compression with moderate volume
lift and low precursor, it identifies when a market is "tightly wound" and
ready to snap.

1. Vertical Ignition

Volume and price are detaching together in a vertical event.
Indicators: High volume lift spike with high price precursor.
Semantic Meaning: Launching/Breakout — the move has ignited.

2. Coiled Compression

Energy is building before the vertical move.
Indicators: Moderate volume lift with low price precursor.
Semantic Meaning: Pre-Pump/Loaded — tightly wound and ready to snap.

3. Organic Trend

Steady momentum without abnormal verticality.
Indicators: Low/steady volume lift with moderate price precursor.
Semantic Meaning: Healthy momentum — supported but not explosive.

4. Faded Exhaustion

The vertical leg has lost its lift.
Indicators: Falling volume lift with flat price precursor.
Semantic Meaning: Leg is dead — the ignition has faded.

# Summary of PumpDump Categories

| Category           | Volume Lift | Price Precursor | Market "Feel"            |
|:-------------------|:------------|:----------------|:-------------------------|
| Vertical Ignition  | High Spike  | High            | Launching / Breakout     |
| Coiled Compression | Moderate    | Low             | Pre-Pump / Loaded        |
| Organic Trend      | Low/Steady  | Moderate        | Healthy Momentum         |
| Faded Exhaustion   | Falling     | Flat            | Leg is Dead              |
*/

/*
Signal identifies pre-pump microstructure from volume lift, price verticality,
and book coiling. It holds no per-pair state: prior measurements in the tree are
the replay source for every baseline, so another process can rebuild the same
state from measurement artifacts keyed by symbol.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
}

type tickerSample struct {
	symbol   string
	bid      float64
	ask      float64
	last     float64
	volume   float64
	change   float64
	changePC float64
	spread   float64
	stamp    float64
}

/*
NewSignal constructs the verticality signal. The tree is the only history store.
*/
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
	return []string{"ticker"}
}

/*
Measure scores ticker rows against tree-backed measurements and enriched book,
trade, and cross-section context.
*/
func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		if signal == nil || datapoint == nil {
			return
		}

		if crossSection == nil {
			errnie.Error(errnie.Err(errnie.Validation, "pumpdump: nil cross-section", nil))

			return
		}

		if datura.Peek[string](datapoint, "channel") != "ticker" {
			return
		}

		for rowIndex := 0; ; rowIndex++ {
			select {
			case <-signal.ctx.Done():
				signal.err = signal.ctx.Err()

				return
			default:
			}

			measurement := signal.measureRow(datapoint, crossSection, rowIndex)

			if measurement == nil {
				if datura.Peek[string](datapoint, "data", rowIndex, "symbol") == "" {
					return
				}

				continue
			}

			if !yield(measurement) {
				return
			}
		}
	}
}

func (signal *Signal) measureRow(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
	rowIndex int,
) *datura.Artifact {
	sample, ok := readTickerSample(datapoint, rowIndex)

	if !ok {
		return nil
	}

	if invariant, anomalous := sample.invalidInvariant(); invariant != "" {
		if anomalous {
			logInvalidRow(sample.symbol, invariant)
		}

		return nil
	}

	row, err := market.SymbolFromTicker(datapoint, rowIndex)

	if err != nil {
		errnie.Error(errnie.Err(errnie.Validation, "pumpdump: ticker row", err).With(
			"symbol", sample.symbol,
		))

		return nil
	}

	if err = crossSection.Observe(row); errnie.Error(err) != nil {
		return nil
	}

	history := signal.history(sample.symbol)
	book := signal.bookEnrichment(sample.symbol, sample.stamp)
	trades := signal.tradeEnrichment(sample.symbol, history.stamps, sample.stamp)
	metrics := signal.metrics(sample, history, book, trades, crossSection)

	measurement := datura.Acquire("pumpdump", datura.APPJSON)
	measurement.WithRole("measurement")
	measurement.WithScope(sample.symbol)
	errnie.Error(measurement.SetOrigin(string(logic.SourcePumpDump)))
	measurement.SetTimestamp(datapoint.Timestamp())

	writeMeasurement(measurement, sample, book, trades, metrics)

	confidence := dist.Write(measurement, classify(metrics))

	if confidence <= 0 {
		measurement.Release()

		return nil
	}

	return measurement
}

func (signal *Signal) metrics(
	sample tickerSample,
	history measurementHistory,
	book bookSnapshot,
	trades tradeSnapshot,
	crossSection *market.CrossSection,
) ignitionMetrics {
	volumeDelta, logReturn := sample.delta(history)
	tradeVolume := trades.volume
	liftSample := tradeVolume

	if liftSample <= 0 {
		liftSample = volumeDelta
	}

	liftBaseline := positiveSamples(history.tradeVolumes)

	if tradeVolume <= 0 {
		liftBaseline = positiveSamples(history.volumeDeltas)
	}

	symbolRVOL := statutil.ScaleByMedianOrUnity(liftSample, liftBaseline)
	peerRVOL := peerRVOL(sample.volume, crossSection)
	rvol := geometricMean(symbolRVOL, peerRVOL)

	symbolPrecursor := statutil.ScaleByMedianOrUnity(
		logReturn,
		positiveSamples(history.logReturns),
	)
	peerPrecursor := peerPrecursor(sample.symbol, logReturn, crossSection)
	precursor := geometricMean(symbolPrecursor, peerPrecursor)

	bookCompression := 0.0
	compression := statutil.InvertedCompression(sample.spread, positiveSamples(history.spreads))

	if book.spread > 0 {
		bookCompression = statutil.InvertedCompression(
			book.spread,
			positiveSamples(history.bookSpreads),
		)
		compression = bookCompression
	}

	return ignitionMetrics{
		rvol:             rvol,
		precursor:        precursor,
		compression:      compression,
		rvolDecline:      rvolDecline(rvol, tradeVolume, history),
		ignitionFloor:    history.ignitionFloor,
		peerRvol:         peerRVOL,
		peerPrecursor:    peerPrecursor,
		bookCompression:  bookCompression,
		compressionFloor: history.compressionFloor,
		declineFloor:     history.declineFloor,
		volumeDelta:      volumeDelta,
		logReturn:        logReturn,
	}
}

func (sample tickerSample) delta(history measurementHistory) (
	volumeDelta, logReturn float64,
) {
	firstObservation := history.prevLast <= 0 || history.prevVolume <= 0

	if !firstObservation {
		volumeDelta = math.Max(0, sample.volume-history.prevVolume)
		logReturn = math.Max(0, math.Log(sample.last/history.prevLast))

		return volumeDelta, logReturn
	}

	volumeDelta = math.Max(0, sample.volume)
	anchorLast := sample.last - sample.change

	if anchorLast <= 0 && sample.changePC != 0 {
		anchorLast = sample.last / (1 + sample.changePC/100)
	}

	if anchorLast > 0 && anchorLast != sample.last {
		logReturn = math.Max(0, math.Log(sample.last/anchorLast))
	}

	return volumeDelta, logReturn
}

func writeMeasurement(
	measurement *datura.Artifact,
	sample tickerSample,
	book bookSnapshot,
	trades tradeSnapshot,
	metrics ignitionMetrics,
) {
	measurement.MergeOutput("rvol", metrics.rvol)
	measurement.MergeOutput("precursor", metrics.precursor)
	measurement.MergeOutput("compression", metrics.compression)
	measurement.MergeOutput("rvolDecline", metrics.rvolDecline)
	measurement.MergeOutput("spread", sample.spread)
	measurement.MergeOutput("bookCompression", metrics.bookCompression)
	measurement.MergeOutput("peerRvol", metrics.peerRvol)
	measurement.MergeOutput("peerPrecursor", metrics.peerPrecursor)

	measurement.Merge("volume", sample.volume)
	measurement.Merge("last", sample.last)
	measurement.Merge("spread", sample.spread)
	measurement.Merge("bookSpread", book.spread)
	measurement.Merge("touchDepth", book.touchDepth)
	measurement.Merge("tradeVolume", trades.volume)
	measurement.Merge("volumeDelta", metrics.volumeDelta)
	measurement.Merge("logReturn", metrics.logReturn)
	measurement.Merge("timestamp", sample.stamp)
}

func readTickerSample(datapoint *datura.Artifact, rowIndex int) (tickerSample, bool) {
	symbol := datura.Peek[string](datapoint, "data", rowIndex, "symbol")

	if symbol == "" {
		return tickerSample{}, false
	}

	bid := datura.Peek[float64](datapoint, "data", rowIndex, "bid")
	ask := datura.Peek[float64](datapoint, "data", rowIndex, "ask")

	return tickerSample{
		symbol:   symbol,
		bid:      bid,
		ask:      ask,
		last:     datura.Peek[float64](datapoint, "data", rowIndex, "last"),
		volume:   datura.Peek[float64](datapoint, "data", rowIndex, "volume"),
		change:   datura.Peek[float64](datapoint, "data", rowIndex, "change"),
		changePC: datura.Peek[float64](datapoint, "data", rowIndex, "change_pct"),
		spread:   ask - bid,
		stamp:    float64(datapoint.Timestamp()),
	}, true
}

/*
invalidInvariant reports why a ticker row cannot be scored, and whether that
reason is anomalous. A missing last/volume is normal for an illiquid pair that
has not traded — skip it quietly. A crossed book (ask < bid) is a genuine data
anomaly worth logging.
*/
func (sample tickerSample) invalidInvariant() (reason string, anomalous bool) {
	if sample.last <= 0 {
		return "last", false
	}

	if sample.volume <= 0 {
		return "volume", false
	}

	if sample.ask < sample.bid {
		return "ask < bid", true
	}

	return "", false
}

func logInvalidRow(symbol, invariant string) {
	errnie.Error(errnie.Err(errnie.Validation, "pumpdump: invalid ticker row", nil).With(
		"symbol", symbol,
		"invariant", invariant,
	))
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
