package leadlag

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
)

/*
LeadLag is the "Anchor" perspective, measuring the temporal correlation
between a leader asset (typically BTC/EUR) and the rest of the market.

1. What it measures exactly (in isolation)

The LeadLag signal measures temporal correlation between the anchor pair
and each follower.

Cross-Lag Correlation: It doesn't just look at if they are moving together,
but by how many bars one is leading the other.

Anchor Threshold: It only activates when the anchor moves significantly
(≥ 0.05%).

Lag Fraction: Measures what percentage of the leader's move the follower has
yet to complete.

---

2. Semantically, what story does it tell?

The LeadLag signal tells the story of market leadership and catch-up
inefficiency.

The "Inefficiency" Story: It finds "free money" by identifying altcoins that
have a high statistical probability of following BTC but haven't "woken up"
yet.

The "Beta Drift" Story: It identifies symbols that have no unique alpha of
their own and are simply being dragged along by the market tide.

1. Inefficient Lag

The follower has not yet caught up to the leader's move.
Indicators: High lead/lag correlation with high lag fraction.
Semantic Meaning: Catch-up opportunity — high-probability follow-through.

2. Synchronized Drift

The follower has already absorbed the leader's move.
Indicators: High lead/lag correlation with low lag fraction.
Semantic Meaning: Systemic beta — the asset is a passenger.

3. Decoupled Move

The follower is moving independently of the anchor.
Indicators: Low lead/lag correlation.
Semantic Meaning: Idiosyncratic alpha — a local catalyst is at play.

4. Anchor Stall

The leader itself has stopped moving.
Indicators: Low lead/lag correlation with low lag fraction.
Semantic Meaning: Leadership exhaustion — the anchor move may be over.

# Summary of LeadLag Categories

| Category           | Lead/Lag Correlation | Lag Fraction | Market "Feel"             |
|:-------------------|:---------------------|:-------------|:--------------------------|
| Inefficient Lag    | High                 | High         | Catch-up Opportunity      |
| Synchronized Drift | High                 | Low          | Systemic Beta             |
| Decoupled Move     | Low                  | N/A          | Idiosyncratic Alpha       |
| Anchor Stall       | Low                  | Low          | Leadership Exhaustion     |
*/
/*
Signal measures temporal correlation between the anchor pair and each follower.
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
	Section     *Section
}

/*
NewSignal composes the lag pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	section, _ := NewSectionFromConfig()
	lagStage := algorithm.NewLag()

	return &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        tree,
		Section:     section,
		algo: nomagique.Number(
			lagStage,
			probability.NewClassifier(
				datura.Acquire("leadlag-classifier", datura.APPJSON).Poke(
					[]string{"inefficient", "sync", "decoupled", "stall"},
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

	result := datura.Acquire("leadlag", datura.APPJSON)
	result.WithRole("measurement")
	result.WithScope("leadlag")

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

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
