package sentiment

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
	"github.com/theapemachine/qpool"
)

/*
Sentiment is the Bullish Breadth perspective, measuring global market
conviction by looking at the behavior of the entire universe simultaneously.

1. What it measures exactly (in isolation)

The Sentiment signal measures global market conviction by looking at the
behavior of the entire universe simultaneously.

Market Breadth: The ratio of symbols with a positive $changePct$ versus the
total number of symbols.

Leadership Performance: Tracks the median performance of the "top" symbols to
see if the leaders are actually leading.

---

2. Semantically, what story does it tell?

The Sentiment signal tells the story of global conviction and rising tides.

The "Rising Tide" Story: It tells you if an asset's move is a solo effort or
if it is being carried by a global "risk-on" regime where every asset is
moving in unison.

The "Conviction" Story: It distinguishes between a "fake" leader move (where
only one asset is up) and a high-conviction market environment (breadth
> 0.55).

1. Risk-On Surge

Broad participation with strong leadership.
Indicators: High breadth (> 0.55) with strong leader performance.
Semantic Meaning: Rising tide/global buy — global risk-on regime.

2. Divergent Move

Leaders are strong but breadth is thin.
Indicators: Low breadth with strong leader performance.
Semantic Meaning: Idiosyncratic alpha — a solo leader effort.

3. Systemic Slump

Both breadth and leadership are weak.
Indicators: Low breadth with weak leader performance.
Semantic Meaning: Global risk-off — systemic slump across the universe.

# Summary of Sentiment Categories

| Category       | Breadth        | Leader Strength | Market "Feel"            |
|:---------------|:---------------|:----------------|:-------------------------|
| Risk-On Surge  | High (>0.55)   | Strong          | Rising Tide / Global Buy |
| Divergent Move | Low            | Strong          | Idiosyncratic Alpha      |
| Systemic Slump | Low            | Weak            | Global Risk-Off          |
*/
/*
Signal measures global market conviction from breadth and leadership performance.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx         context.Context
	cancel      context.CancelFunc
	err         error
	pool        *qpool.Q[any]
	subscribers *sync.Map
	algo        io.ReadWriteCloser
	tree        *dmt.Tree
}

/*
NewSignal composes the conviction pipeline for tree replay measurement.
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
			equation.NewConviction(), probability.NewClassifier(
				datura.Acquire("sentiment-classifier", datura.APPJSON).Poke(
					[]string{"surgeScore", "divergentScore", "slumpScore"},
					"inputs",
				),
			),
		),
	}

	return signal
}

func (signal *Signal) Measure(query *datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	var frame *datura.Artifact

	samples := 0

	for _, role := range []string{"ticker", "book", "trade", "ohlc"} {
		probe := datura.Acquire("trader", datura.APPJSON)
		probe.WithRole(role)
		probe.WithScope(scope)

		for stored := range signal.tree.Seek(probe.Prefix("role", "scope")) {
			packed, err := stored.Message().MarshalPacked()

			stored.Release()

			if errnie.Error(err) != nil {
				continue
			}

			replay := datura.Acquire("trader", datura.APPJSON)

			if _, err := replay.Write(packed); errnie.Error(err) != nil {
				replay.Release()
				continue
			}

			errnie.Error(transport.NewFlipFlop(replay, signal.algo))
			samples++

			if frame != nil {
				frame.Release()
			}

			frame = replay
		}

		probe.Release()
	}

	result := datura.Acquire("sentiment", datura.APPJSON)
	result.WithRole("measurement")
	result.WithScope("sentiment")

	if frame == nil || samples == 0 {
		result.WithPayload([]byte("{}"))
		return result
	}

	payload := frame.DecryptPayload()

	frame.Release()

	if len(payload) == 0 {
		result.WithPayload([]byte("{}"))
		return result
	}

	result.WithPayload(payload)
	result.Merge("samples", float64(samples))
	result.Merge("calibrated", true)

	return result
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
