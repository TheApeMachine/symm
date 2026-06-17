package leadlag

import (
	"context"
	"io"
	"sync"
	"time"

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
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	surpriseTree *dmt.Tree
	Section      *Section
	measureScope string
	trade        *feed.Trade
	ticker       *feed.Ticker
}

/*
NewSignal composes the lag pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	section, _ := NewSectionFromConfig()
	lagStage := algorithm.NewLag()
	surpriseTree, _ := dmt.NewTree("")

	tradeFeed := feed.NewTrade(ctx)
	tickerFeed := feed.NewTicker(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		surpriseTree: surpriseTree,
		Section:      section,
		trade:        tradeFeed,
		ticker:       tickerFeed,
		algo: nomagique.Number(
			lagStage,
			probability.NewClassifier(
				lagStage.InefficientReading(),
				lagStage.SyncReading(),
				lagStage.DecoupledReading(),
				lagStage.StallReading(),
			),
			probability.NewDMTSurprise(
				surpriseTree,
				5,
			),
		),
	}

	tradeFeed.OnUpdate = func(record *feed.TradeRecord) {
		if record == nil || record.Price <= 0 {
			return
		}

		signal.Section.ObservePrice(record.Symbol, record.Price, record.Timestamp)
	}

	tickerFeed.OnUpdate = func(record *feed.TickerRecord) {
		if record == nil {
			return
		}

		price := record.Last

		if price <= 0 {
			price = (record.Ask + record.Bid) / 2
		}

		if price <= 0 {
			return
		}

		signal.Section.ObservePrice(record.Symbol, price, record.Timestamp)
	}

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
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
	signal.ticker.Scope = scope
	signal.trade.ResetReadHead()
	signal.ticker.ResetReadHead()

	out := datura.Acquire("leadlag-out", datura.Artifact_Type_json).WithScope(scope)

	if out == nil {
		return logic.Measurement{}, nil
	}

	errnie.Does(func() (int64, error) {
		return transport.Copy(
			signal.algo,
			io.MultiReader(signal.trade, signal.ticker, signal),
		)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.IO, "failed to copy to algo", err))
	})

	if err := transport.NewFlipFlop(out, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	strength := datura.Peek[float64](out, "lag.strength")

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

	snapshot := signal.Section.Features(scope)
	observedAt := snapshot.ObservedAt

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	return logic.Measurement{
		Source:     logic.SourceLeadLag,
		Symbol:     scope,
		Price:      snapshot.Price,
		Strength:   strength,
		Category:   leadlagCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   logic.PositionTypeNone,
		Confidence: confidence,
		Surprise:   datura.Peek[float64](out, "transition.surprise"),
		ObservedAt: observedAt,
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
	snapshot := signal.Section.Features(scope)

	if snapshot.Price <= 0 {
		return nil
	}

	isAnchor := 0.0

	if snapshot.IsAnchor {
		isAnchor = 1
	}

	moveReady := 0.0

	if snapshot.MoveReady {
		moveReady = 1
	}

	moveMoved := 0.0

	if snapshot.MoveMoved {
		moveMoved = 1
	}

	lagOK := 0.0

	if snapshot.LagOK {
		lagOK = 1
	}

	contempOK := 0.0

	if snapshot.ContempOK {
		contempOK = 1
	}

	artifact := datura.Acquire("lag-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(feed.EncodePayload(
		isAnchor,
		snapshot.Price,
		moveReady,
		moveMoved,
		snapshot.StallMargin,
		lagOK,
		float64(snapshot.LagBars),
		snapshot.LagCorr,
		contempOK,
		snapshot.ContempCorr,
		float64(snapshot.SampleCount),
	))

	return artifact
}

func leadlagCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryInefficientLag
	case 2:
		return logic.CategorySynchronizedDrift
	case 3:
		return logic.CategoryDecoupledMove
	case 4:
		return logic.CategoryAnchorStall
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
