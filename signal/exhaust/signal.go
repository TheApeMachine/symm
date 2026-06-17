package exhaust

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
	"github.com/theapemachine/symm/logic"
	feed "github.com/theapemachine/symm/signal"
)

/*
Exhaust is the "Exit Thesis" perspective, tracking microstructure decay to
advise on the urgency of closing an open position.

1. What it measures exactly (in isolation)

The Exhaust signal tracks microstructure decay to advise on the urgency of
closing an open position. Unlike entry signals that look for momentum
ignition, Exhaust looks for momentum rot.

Book Thinning: Measures the trend of bid/ask depth; if depth is disappearing
as price moves, the move is "hollow".

Pressure Fade: Tracks the decay in trade pressure EMA; it signals when the
aggressive "hitters" have run out of ammunition.

Spread Widening: Monitors the bid/ask spread; widening spreads during a trend
indicate increasing mechanical resistance and risk.

Imbalance Flip: Detects when the "weight" of the book flips from the support
side to the resistance side.

---

2. Semantically, what story does it tell?

The Exhaust signal tells the story of when to leave — momentum rot and thesis
decay.

The "Party is Over" Story: It identifies the exact moment a trend stops being
supported by fresh liquidity and begins to rely on "fumes."

The "Trap" Story: By spotting Book Thinning while price is still rising, it
warns that the bid wall is crumbling and a sharp reversal is imminent.

The "Thesis Fade" Story: It provides a "Thesis-decay" exit — if the reason you
entered (imbalance and pressure) is gone, it closes the trade even if the
stop-loss hasn't been hit yet.

1. Mechanical Collapse

Defensive depth is crumbling at the touch.
Indicators: Book thinning as the primary metric.
Semantic Meaning: Crumbling walls/flash-risk — urgent exit.

2. Thermal Exhaustion

Aggressive trade pressure is fading.
Indicators: Pressure fade as the primary metric.
Semantic Meaning: Dying momentum/topping out — soft exit.

3. Fragile Expansion

Spread is widening against the trend.
Indicators: Spread widen as the primary metric.
Semantic Meaning: Increasing friction/risky hold — monitor closely.

4. Active Reversal

Book imbalance has flipped to the other side.
Indicators: Imbalance flip as the primary metric.
Semantic Meaning: Sentiment flip/counter-attack — urgent exit.

# Summary of Exhaust Categories

| Category            | Primary Metric    | Urgency  | Market "Feel"                        |
|:--------------------|:------------------|:---------|:-------------------------------------|
| Mechanical Collapse | Book Thinning     | High     | Crumbling Walls / Flash-Risk         |
| Thermal Exhaustion  | Pressure Fade     | Moderate | Dying Momentum / Topping Out         |
| Fragile Expansion   | Spread Widen      | Moderate | Increasing Friction / Risky Hold     |
| Active Reversal     | Imbalance Flip    | High     | Sentiment Flip / Counter-Attack      |
*/
/*
Signal classifies microstructure decay modes that advise when to close a position.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	surpriseTree *dmt.Tree
	crossSection *CrossSection
	measureScope string
	book         *feed.Book
	trade        *feed.Trade
	ticker       *feed.Ticker
}

/*
NewSignal composes the decay pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	crossSection := NewCrossSection(featureRingCapacity)
	surpriseTree, _ := dmt.NewTree("")
	decay := algorithm.NewDecay()

	bookFeed := feed.NewBook(ctx)
	tradeFeed := feed.NewTrade(ctx)
	tickerFeed := feed.NewTicker(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		surpriseTree: surpriseTree,
		crossSection: crossSection,
		book:         bookFeed,
		trade:        tradeFeed,
		ticker:       tickerFeed,
		algo: nomagique.Number(
			decay,
			probability.NewClassifier(
				decay.MechanicalReading(),
				decay.FragileReading(),
				decay.ThermalReading(),
				decay.ReversalReading(),
			),
			probability.NewDMTSurprise(
				surpriseTree,
				5,
			),
		),
	}

	bookFeed.OnUpdate = crossSection.observeBook
	tradeFeed.OnUpdate = crossSection.observeTrade
	tickerFeed.OnUpdate = crossSection.observeTick

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "book":
		signal.book.Update(artifact)
	case "trade":
		signal.trade.Update(artifact)
	case "ticker":
		signal.ticker.Update(artifact)
	case "measurement":
		signal.Measure(artifact)
	}

	return nil
}

func (signal *Signal) Measure(in *datura.Artifact) (logic.Measurement, error) {
	scope := datura.Peek[string](in, "scope")
	signal.measureScope = scope
	signal.trade.Scope = scope
	signal.book.Scope = scope
	signal.ticker.Scope = scope
	signal.trade.ResetReadHead()
	signal.book.ResetReadHead()
	signal.ticker.ResetReadHead()

	out := datura.Acquire("exhaust-out", datura.Artifact_Type_json).WithScope(scope)

	if out == nil {
		return logic.Measurement{}, nil
	}

	errnie.Does(func() (int64, error) {
		return transport.Copy(
			signal.algo,
			io.MultiReader(signal.trade, signal.book, signal.ticker, signal),
		)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.IO, "failed to copy to algo", err))
	})

	if err := transport.NewFlipFlop(out, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := datura.Peek[float64](out, "decay.urgency")

	if strength <= 0 {
		return logic.Measurement{}, nil
	}

	categoryIndex := datura.Peek[int](out, "classifier.category")

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	confidence := datura.Peek[float64](out, "classifier.confidence")

	if !logic.ScalarFinite(confidence) || confidence <= 0 {
		return logic.Measurement{}, nil
	}

	snapshot := signal.trade.Snapshot(scope)

	price := snapshot.Price

	if price <= 0 {
		payload, payloadOK := signal.crossSection.payload(scope)

		if payloadOK && len(payload) > 0 {
			price = payload[0]
		}
	}

	return logic.Measurement{
		Source:     logic.SourceExhaustion,
		Symbol:     scope,
		Price:      price,
		Strength:   strength,
		Volume:     snapshot.Volume,
		Spread:     signal.book.Spread(scope),
		Elapsed:    snapshot.Elapsed,
		Category:   exhaustCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](out, "transition.surprise"),
		ObservedAt: snapshot.Observed,
	}.UnlessPublishable(), nil
}

func (signal *Signal) Read(buffer []byte) (int, error) {
	artifact := signal.featureArtifact(signal.measureScope)

	if artifact == nil {
		return 0, io.EOF
	}

	return feed.ReadFeatureArtifact(buffer, artifact)
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	payload, ok := signal.crossSection.payload(scope)

	if !ok || len(payload) == 0 {
		return nil
	}

	artifact := datura.Acquire("decay-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(feed.EncodePayload(payload...))

	return artifact
}

func exhaustCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryMechanicalCollapse
	case 2:
		return logic.CategoryFragileExpansion
	case 3:
		return logic.CategoryThermalExhaustion
	case 4:
		return logic.CategoryActiveReversal
	default:
		return logic.CategoryTypeNone
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	if signal.surpriseTree != nil {
		_ = signal.surpriseTree.Close()
	}

	return nil
}

