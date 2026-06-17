package pumpdump

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
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriter
	surpriseTree *dmt.Tree
	crossSection *CrossSection
	measureScope string
	trade        *feed.Trade
	book         *feed.Book
}

/*
NewSignal composes the verticality pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	verticality, verticalityErr := algorithm.NewVerticality()

	if verticalityErr != nil {
		cancel()

		return nil
	}

	crossSection := NewCrossSection(time.Minute, 0)
	surpriseTree, _ := dmt.NewTree("")
	tradeFeed := feed.NewTrade(ctx)
	bookFeed := feed.NewBook(ctx)

	signal := &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		surpriseTree: surpriseTree,
		crossSection: crossSection,
		trade:        tradeFeed,
		book:         bookFeed,
		algo: nomagique.Number(
			verticality,
			probability.NewClassifier(
				verticality.IgnitionReading(),
				verticality.CompressionReading(),
				verticality.TrendReading(),
				verticality.ExhaustionReading(),
			),
			probability.NewDMTSurprise(
				surpriseTree,
				5,
			),
		),
	}

	tradeFeed.OnUpdate = func(record *feed.TradeRecord) {
		_ = crossSection.observeTrade(record)
	}

	bookFeed.OnUpdate = func(record *feed.BookRecord) {
		_ = crossSection.observeBook(record)
	}

	return signal
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "book":
		signal.book.Update(artifact)
	case "trade":
		signal.trade.Update(artifact)
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
	signal.trade.ResetReadHead()
	signal.book.ResetReadHead()

	out := datura.Acquire("pumpdump-out", datura.Artifact_Type_json).WithScope(scope)

	if out == nil {
		return logic.Measurement{}, nil
	}

	errnie.Does(func() (int64, error) {
		return transport.Copy(
			signal.algo,
			io.MultiReader(signal.trade, signal.book, signal),
		)
	}).Or(func(err error) {
		errnie.Error(errnie.Err(errnie.IO, "failed to copy to algo", err))
	})

	if err := transport.NewFlipFlop(out, signal.algo); err != nil {
		return logic.Measurement{}, errnie.Error(err)
	}

	snapshot := signal.scopeSnapshot(scope)

	categoryIndex := datura.Peek[int](out, "classifier.category")

	if categoryIndex == 0 {
		categoryIndex = int(datura.Peek[float64](out, "verticality.category"))
	}

	if categoryIndex == 0 {
		return logic.Measurement{}, nil
	}

	strength := datura.Peek[float64](out, "verticality.strength")

	if strength <= 0 {
		return logic.Measurement{}, nil
	}

	position := logic.PositionTypeNone

	if snapshot.Move > 0 {
		position = logic.PositionTypeLong
	}

	if snapshot.Move < 0 {
		position = logic.PositionTypeShort
	}

	return logic.Measurement{
		Source:     logic.SourcePumpDump,
		Symbol:     scope,
		Price:      snapshot.Price,
		Strength:   strength,
		Volume:     snapshot.Volume,
		Spread:     snapshot.Spread,
		Elapsed:    snapshot.Elapsed,
		Category:   pumpDumpCategory(categoryIndex),
		Regime:     logic.RegimeTypeNone,
		Position:   position,
		Confidence: datura.Peek[float64](out, "classifier.confidence"),
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

func (signal *Signal) scopeSnapshot(scope string) ScopeSnapshot {
	snapshot := TradeScopeSnapshot(signal.trade, scope)

	if snapshot.Price > 0 {
		return snapshot
	}

	return BookScopeSnapshot(signal.book, scope)
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	if !signal.crossSection.Ready(scope) {
		return nil
	}

	snapshot := signal.scopeSnapshot(scope)

	if snapshot.Price <= 0 || snapshot.Spread <= 0 {
		return nil
	}

	payload, ok := signal.crossSection.verticalityPayload(
		scope,
		snapshot.Move,
		snapshot.Precursor,
	)

	if !ok || len(payload) == 0 {
		return nil
	}

	artifact := datura.Acquire("verticality-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(feed.EncodePayload(payload...))

	return artifact
}

func pumpDumpCategory(categoryIndex int) logic.CategoryType {
	switch categoryIndex {
	case 1:
		return logic.CategoryVerticalIgnition
	case 2:
		return logic.CategoryCoiledCompression
	case 3:
		return logic.CategoryOrganicTrend
	case 4:
		return logic.CategoryFadedExhaustion
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
