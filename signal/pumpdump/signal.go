package pumpdump

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/nomagique/vector"
	"github.com/theapemachine/qpool"
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
					"pumpdump", datura.APPJSON,
				).WithAttributes(map[string]any{
					"ticker": map[string]any{
						"root": "data",
						"inputs": []string{
							"symbol",
							"bid",
							"bid_qty",
							"ask",
							"ask_qty",
							"last",
							"volume",
							"vwap",
							"low",
							"high",
							"change",
							"change_pct",
							"timestamp",
						},
						"transforms": map[string]string{
							"volume": "ema",
							"vwap":   "ema",
						},
					},
					"book": map[string]any{
						"root": "data",
						"inputs": []string{
							"bids",
							"asks",
							"timestamp",
						},
					},
					"ohlc": map[string]any{
						"root": "data",
						"inputs": []string{
							"symbol",
							"open",
							"high",
							"low",
							"close",
							"trades",
							"volume",
							"interval_begin",
							"interval",
							"timestamp",
						},
					},
					"trade": map[string]any{
						"root": "data",
						"inputs": []string{
							"symbol",
							"side",
							"price",
							"qty",
							"ord_type",
							"trade_id",
							"timestamp",
						},
					},
				}),
			),
			equation.NewIgnition(
				datura.Acquire("pumpdump-ignition", datura.APPJSON).WithAttributes(datura.Map[any]{
					"order":     []string{"rvol", "precursor", "compression"},
					"outputs":   []string{"ignition", "compression", "trend", "exhaustion"},
					"threshold": 0.0,
					"inputs": map[string]any{
						"rvol": map[string]any{
							"input":       "volume",
							"useDelta":    1.0,
							"shortWindow": 0.0,
							"longWindow":  0.0,
							"outputKey":   "rvol",
							"scale":       0.0,
						},
						"precursor": map[string]any{
							"input":        "last",
							"returnLag":    1.0,
							"longWindow":   0.0,
							"positiveOnly": 1.0,
							"outputKey":    "precursor",
							"stageIndex":   1.0,
							"scale":        0.0,
						},
						"compression": map[string]any{
							"source": "value",
							"scale":  0.0,
						},
						"spread": map[string]any{
							"inputs": []string{"bid", "ask"},
						},
						"joint": map[string]any{
							"leftKey":        "rvol",
							"rightKey":       "precursor",
							"destinationKey": "ignition",
							"source":         "ignition",
							"output":         "ignition",
						},
					},
				}),
			),
			probability.NewClassifier(
				datura.Acquire(
					"pumpdump-classifier", datura.APPJSON,
				).WithAttributes(datura.Map[any]{
					"inputs": []string{
						"ignition", "compression", "trend", "exhaustion",
					},
				}),
			),
		),
	}

	return signal
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	scope, _ := datapoint.Scope()

	state := datura.Acquire(
		"pumpdump", datura.APPJSON,
	).WithRole(
		"measurement",
	).WithScope(
		scope,
	).WithPayload(
		datapoint.DecryptPayload(),
	)

	if errnie.Error(transport.NewFlipFlop(
		state, signal.algo,
	)) != nil {
		state.Release()
		return nil
	}

	return state
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
