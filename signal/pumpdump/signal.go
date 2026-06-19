package pumpdump

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
		"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/vector"
	"github.com/theapemachine/qpool"
	symmsignal "github.com/theapemachine/symm/signal"
)

/*
PumpDump is the Ignition perspective, identifying pre-pump microstructure by
looking for sudden "verticality" in volume and price.

1. What it measures exactly (in isolation)

The PumpDump signal identifies pre-pump microstructure by looking for sudden
"verticality" in volume and price.

Volume Lift (RVOL): Measures fast and medium-term volume spikes against a
median hourly baseline.

Precursor Move: Uses a $PositiveMove$ dynamic to score how much the price has
already begun to detach from its recent anchor.

Spread Compression: Scores how much the bid/ask spread has tightened versus
its own baseline.

Move Classifier: A state-free primitive that maps these metrics into an
explicit "Pump" or "Dump" class.

---

2. Semantically, what story does it tell?

The PumpDump signal tells the story of explosive ignition and coiled energy.

The "Ignition" Story: It identifies the exact moment a move stops being random
walk and becomes a vertical event driven by abnormal volume "lift".

The "Coiled Spring" Story: By tracking spread compression and book-side
strength, it identifies when a market is "tightly wound" and ready to snap.

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
Signal identifies pre-pump microstructure from volume lift and price verticality.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriter
	tree        *dmt.Tree
}

/*
NewSignal composes the verticality pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			vector.NewFeatureExtractor(
				datura.Acquire(
					"verticality-extract", datura.APPJSON,
				).Poke(
					"data", "root",
				).Poke([]string{
					"volume", "vwap", "last", "bid", "ask", "change_pct",
				}, "order").Poke(map[string]any{
					"volume":     map[string]any{"transform": "ema"},
					"vwap":       map[string]any{"transform": "ema"},
					"last":       map[string]any{},
					"bid":        map[string]any{},
					"ask":        map[string]any{},
					"change_pct": map[string]any{},
				}, "inputs"),
			),
			equation.NewVerticality(),
			probability.NewClassifier(
				datura.Acquire(
					"pumpdump-classifier", datura.APPJSON,
				).Poke(
					[]string{
						"ignition", "compression", "trend", "exhaustion",
					},
					"inputs",
				),
			),
		),
	}

	return signal
}

func (signal *Signal) Measure(query *datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	if scope == "" {
		return nil
	}

	symmsignal.ReplayScopeIngest(signal.tree, scope, query, signal.algo)

	if datura.Peek[int](query, "classifier", "category") <= 0 {
		return nil
	}

	symmsignal.PublishMeasurement(signal.tree, "pumpdump", query)

	return query
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
