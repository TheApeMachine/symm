package depthflow

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
DepthFlow is the "Weight of the Book" perspective, measuring the asymmetry of
intent by looking at multiple levels of the order book, weighted by their
distance from the mid-price.

1. What it measures exactly (in isolation)

The DepthFlow signal measures distance-decayed book imbalance with
trade-pressure confirmation.

Weighted Depth Imbalance (WBI): Applies an exponential decay kernel
(exp(-λ · d)) to levels. Deep "spoof" walls are down-weighted, while
liquidity near the touch is prioritized.

Toxic Filter: It actively subtracts "toxic" levels — large, young blocks near
the touch that are frequently cancelled rather than filled — from the
imbalance calculation.

Trade Pressure EMA: Integrates recent trade sides into a running pressure
index to see if the book imbalance is actually resulting in trades.

Spoof Skew: Specifically flags when deep-book volume contradicts the touch
(e.g., a massive buy wall exists while the top-of-book is being sold into).

---

2. Semantically, what story does it tell?

The DepthFlow signal tells the story of structural gravity and manipulation
in the resting book.

The "Structural Wall" Story: It identifies when the "gravity" of the book is
pulling price in a certain direction.

The "Spoofing" Story: Using the SpoofSkew metric, it warns the engine when a
side is trying to "fake" depth to lure other participants into a trap.

The "Book Decay" Story: By tracking book_thinning and spread_widen events, it
identifies when a side's defensive walls are crumbling.

1. Loaded Imbalance

The book's weight agrees with trade pressure.
Indicators: High WBI with high, confirming trade pressure.
Semantic Meaning: Structural gravity — the wall is real and directional.

2. Spoof Trap

Deep-book shape contradicts what trades are doing.
Indicators: High WBI with low or contradicting trade pressure.
Semantic Meaning: Manipulated/Fake — a bluff wall near the touch.

3. Book Thinning

Defensive depth is disappearing at the touch.
Indicators: Rapidly falling WBI with variable trade pressure.
Semantic Meaning: Exhaustion/Crumbling — hollow, fragile support.

4. Dense Neutrality

Both sides carry balanced, thick depth.
Indicators: Balanced WBI with low trade pressure.
Semantic Meaning: Robust stability — two-sided, sincere liquidity.

# Summary of DepthFlow Categories

| Category         | WBI (Weighted Imbalance) | Trade Pressure    | Market "Feel"            |
|:-----------------|:-------------------------|:------------------|:-------------------------|
| Loaded Imbalance | High                     | High (Agrees)     | Structural Gravity       |
| Spoof Trap       | High                     | Low (Contradicts) | Manipulated/Fake           |
| Book Thinning    | Rapidly Falling          | Variable          | Exhaustion/Crumbling       |
| Dense Neutrality | Balanced                 | Low               | Robust Stability           |
*/
/*
Signal measures distance-decayed book imbalance with trade-pressure confirmation.
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
NewSignal composes the bookflow pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	bookflow := equation.NewBookflow()

	return &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		algo: nomagique.Number(
			bookflow,
			probability.NewClassifier(
				datura.Acquire("depthflow-classifier", datura.APPJSON).Poke(
					[]string{"loadedScore", "spoofScore", "thinScore", "neutralScore"},
					"inputs",
				),
			),
		),
	}
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

	result := datura.Acquire("depthflow", datura.APPJSON)
	result.WithRole("measurement")
	result.WithScope("depthflow")

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
