package pumpdump

import (
	"context"
	"io"
	"math"
	"strconv"
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

Volume Lift (RVOL): Measures positive volume delta spikes against a
median-scaled baseline (short/long windows derived from tick cadence).

Precursor Move: Scores upward price detachment from its recent anchor
(positive-only log-return z-score).

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

	schema := datura.Acquire("pumpdump", datura.APPJSON).WithAttributes(datura.Map[any]{
		"required": []string{"ticker"},
		"ticker": datura.Map[any]{
			"root": "data",
			"inputs": []string{
				"symbol",
				"bid",
				"ask",
				"last",
				"volume",
			},
		},
	})

	ignition := datura.Acquire("pumpdump-ignition", datura.APPJSON).WithAttributes(datura.Map[any]{
		"root": "output",
		"inputs": []string{
			"rvol", "precursor", "compression", "spread", "ignition", "value", "rvolDecline",
		},
		"stageIndex": 0.0,
		"order":      []string{"rvol", "precursor", "compression"},
		"outputs":    []string{"ignition", "compression", "trend", "exhaustion"},
		"threshold":  0.0,
		"rvol": datura.Map[any]{
			"input":       "volume",
			"transform":   "deltaPositive",
			"shortWindow": 0.0,
			"longWindow":  0.0,
			"outputKey":   "rvol",
			"scale":       0.0,
			"scaleMode":   "median",
			"leftKey":     "rvol",
			"rightKey":    "precursor",
			"decline": datura.Map[any]{
				"output": "rvolDecline",
			},
		},
		"precursor": datura.Map[any]{
			"input":        "last",
			"returnLag":    0.0,
			"longWindow":   0.0,
			"positiveOnly": 1.0,
			"outputKey":    "precursor",
			"stageIndex":   1.0,
			"scale":        0.0,
			"scaleMode":    "median",
			"leftKey":      "rvol",
			"rightKey":     "precursor",
		},
		"compression": datura.Map[any]{
			"input":      "spread",
			"outputKey":  "compression",
			"scale":      0.0,
			"scaleMode":  "median",
			"terms":      []string{"compression", "precursor", "rvol"},
			"inverts":    []string{"precursor", "rvol"},
			"gate":       "precursor",
			"gateInvert": 1.0,
			"scaleWire":  "spread",
			"leftKey":    "rvol",
			"rightKey":   "precursor",
		},
		"spread": datura.Map[any]{
			"inputs": []string{"bid", "ask"},
		},
		"ignition": datura.Map[any]{
			"terms":     []string{"rvol", "precursor"},
			"source":    "ignition",
			"combine":   "ratio",
			"leftKey":   "rvol",
			"rightKey":  "precursor",
			"scaleMode": "median",
		},
		"trend": datura.Map[any]{
			"terms":   []string{"precursor", "compression", "rvol"},
			"inverts": []string{"compression"},
		},
		"exhaustion": datura.Map[any]{
			"terms":   []string{"rvol", "precursor"},
			"inverts": []string{"rvol", "precursor"},
			"gate":    "rvolDecline",
		},
		"decline": datura.Map[any]{
			"source":    "rvolDecline",
			"output":    "exhaustion",
			"squash":    0.0,
			"attenuate": []string{"compression"},
		},
		"joint": datura.Map[any]{
			"leftKey":        "rvol",
			"rightKey":       "precursor",
			"destinationKey": "ignition",
			"source":         "ignition",
			"output":         "ignition",
			"combine":        "ratio",
			"scaleMode":      "median",
		},
	})

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			vector.NewFeatureExtractor(schema),
			equation.NewIgnition(ignition),
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

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"pumpdump: signal or datapoint or algo is nil",
			nil,
		))
		return nil
	}

	channel := datura.Peek[string](datapoint, "channel")

	if channel != "" && channel != "ticker" {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"pumpdump: channel mismatch"+channel,
			nil,
		))
		return nil
	}

	if transport.NewFlipFlop(
		datapoint, signal.algo,
	) != nil {
		return nil
	}

	confidence := datura.Peek[float64](datapoint, "output", "confidence")
	uniformConfidence := 1.0 / 4.0

	if confidence <= 0 ||
		math.IsNaN(confidence) ||
		math.IsInf(confidence, 0) ||
		confidence <= uniformConfidence+1e-12 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"pumpdump: confidence too low"+strconv.FormatFloat(confidence, 'f', -1, 64),
			nil,
		))
		return nil
	}

	rvol := datura.Peek[float64](datapoint, "output", "rvol")

	if math.IsNaN(rvol) || math.IsInf(rvol, 0) {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"pumpdump: rvol non-finite"+strconv.FormatFloat(rvol, 'f', -1, 64),
			nil,
		))

		return nil
	}

	return datapoint
}

func (signal *Signal) Error() error {
	return errnie.Error(signal.err)
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return errnie.Error(err)
}
