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
Signal composes the role objects that own raw ticker, book, and trade artifact
history. Signal routes market artifacts and supplies role context to Ticker.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
	ticker *Ticker
	book   *Book
	trade  *Trade
}

/*
NewSignal constructs the verticality signal. The tree is the only history store.
*/
func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	book := NewBook()
	trade := NewTrade()

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		ticker: NewTicker(),
		book:   book,
		trade:  trade,
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker", "book", "trade"}
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
		role := datura.Peek[string](datapoint, "role")
		data := datura.Peek[[]any](datapoint, "data")

		for _, item := range data {
			row, ok := item.(map[string]any)

			if !ok {
				if !yield(datapoint.WithError(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"pumpdump: row object required",
					nil,
				)))) {
					return
				}

				continue
			}

			symbol, ok := row["symbol"].(string)

			if !ok {
				if !yield(datapoint.WithError(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"pumpdump: row symbol required",
					nil,
				)))) {
					return
				}

				continue
			}

			if role == "ticker" {
				price, _ := row["last"].(float64)
				if price <= 0 || math.IsNaN(price) || math.IsInf(price, 0) {
					continue
				}
			}

			rowArtifact := datura.Acquire(
				"pumpdump", datura.APPJSON,
			).WithRole(
				"measurement",
			).WithScope(
				symbol,
			).WithPayload(
				datura.Map[any](row).Marshal(),
			)
			rowArtifact.SetTimestamp(datapoint.Timestamp())
			errnie.Error(rowArtifact.SetOrigin(string(logic.SourcePumpDump)))

			switch role {
			case "ticker":
				measurement := signal.ticker.Measure(rowArtifact, crossSection)
				if measurement == nil {
					continue
				}

				if !yield(measurement) {
					return
				}
			case "book":
				measurement := signal.book.Measure(rowArtifact, crossSection)
				if measurement == nil {
					continue
				}

				if !yield(measurement) {
					return
				}
			case "trade":
				measurement := signal.trade.Measure(rowArtifact, crossSection)
				if measurement == nil {
					continue
				}

				if !yield(measurement) {
					return
				}
			}
		}
	}
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
