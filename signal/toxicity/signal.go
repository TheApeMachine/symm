package toxicity

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	feed "github.com/theapemachine/symm/signal"
)

/*
Toxicity is the Quality perspective, analyzing the "honesty" of the book by
tracking how makers behave when a trade approaches.

1. What it measures exactly (in isolation)

The Toxicity signal analyzes the "honesty" of the book by tracking how makers
behave when a trade approaches.

Cancel-to-Fill Asymmetry: Measures the ratio of liquidity being "pulled"
(cancelled) versus liquidity being "hit" (filled).

Toxic Level Detection: Flags large, young, near-touch blocks that disappear
rather than fill — this is the signature of a bluff.

Directional BookFlow: Emits a directional read based on which side of the book
is "retreating" (vacuum effect).

---

2. Semantically, what story does it tell?

The Toxicity signal tells the story of sincere versus fake liquidity.

The "Bluffing" Story: It exposes makers who are "fake-bidding" to create an
illusion of support, warning the engine that a wall is not "real" and will
crumble upon contact.

The "Vacuum" Story: It identifies a "liquidity vacuum" where one side pulls
away so aggressively that the resulting void "sucks" the price in that
direction.

1. Liquidity Vacuum

One side is retreating and creating a void.
Indicators: High cancel/fill asymmetry with one side retracting.
Semantic Meaning: Vacuum surcharge — the void itself drives price.

2. Toxic Bluff

Near-touch blocks disappear rather than fill.
Indicators: High cancel/fill ratio at near-touch levels.
Semantic Meaning: Manipulated/fake — a bluff wall about to crumble.

3. Hard Support

Liquidity fills rather than cancels on approach.
Indicators: Low cancel/fill ratio (high fill rate) with no side retracting.
Semantic Meaning: Robust/sincere — the wall will hold on contact.

# Summary of Toxicity Categories

| Category         | Cancel/Fill Ratio | Side Retracting | Market "Feel"          |
|:-----------------|:------------------|:----------------|:-----------------------|
| Liquidity Vacuum | High Asymmetry    | One Side        | Vacuum Surcharge       |
| Toxic Bluff      | High              | Near-Touch      | Manipulated / Fake     |
| Hard Support     | Low (High Fill)   | None            | Robust / Sincere       |
*/
/*
Signal analyzes book honesty from cancel-to-fill asymmetry and toxic level detection.
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
NewSignal composes the book-quality pipeline for tree replay measurement.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	bookQuality := algorithm.NewBookQuality()

	signal := &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        dmt.NewTree(""),
		algo: nomagique.Number(
			bookQuality,
			probability.NewClassifier(
				bookQuality.BluffReading(),
				bookQuality.VacuumReading(),
				bookQuality.SupportReading(),
			),
		),
	}

	return signal
}

func (signal *Signal) Measure(query datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	var measurement *datura.Artifact

	for _, role := range []string{"book", "order"} {
		prefix := role + "/" + scope

		for inbound := range signal.tree.Seek([]byte(prefix)) {
			processed := datura.Acquire("toxicity", datura.APPJSON)

			if processed == nil {
				continue
			}

			payload, payloadOK := feed.ArtifactPayload(inbound)

			if !payloadOK {
				processed.Release()
				continue
			}

			if !feed.ValidFloatPayload(payload, feed.BookQualityMinFloats) {
				processed.Release()
				continue
			}

			if processed.WithPayload(payload) == nil {
				processed.Release()
				continue
			}

			if flipErr := transport.NewFlipFlop(processed, signal.algo); flipErr != nil {
				_ = processed.WithError(flipErr)
			}

			if datura.Peek[int](processed, "classifier.category") <= 0 {
				processed.Release()
				continue
			}

			if datura.Peek[float64](processed, "classifier.confidence") <= 0 {
				processed.Release()
				continue
			}

			processed.WithRole("measurement")
			processed.WithScope(scope)

			measurement = processed
		}
	}

	if measurement != nil {
		feed.InsertMeasurement(signal.tree, measurement)
	}

	return measurement
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
