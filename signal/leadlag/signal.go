package leadlag

import (
	"context"
	"encoding/json"
	"io"
	"sync"
	"time"

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
NewSignal composes the lag pipeline and subscribes to market channels.
*/
func NewSignal(
	ctx context.Context,
	pool *qpool.Q[any],
) *Signal {
	ctx, cancel := context.WithCancel(ctx)

	section, _ := NewSectionFromConfig()
	lagStage := algorithm.NewLag()

	return &Signal{
		ctx:         ctx,
		cancel:      cancel,
		pool:        pool,
		subscribers: &sync.Map{},
		tree:        dmt.NewTree(""),
		Section:     section,
		algo: nomagique.Number(
			lagStage,
			probability.NewClassifier(
				lagStage.InefficientReading(),
				lagStage.SyncReading(),
				lagStage.DecoupledReading(),
				lagStage.StallReading(),
			),
		),
	}
}

func (signal *Signal) Update(artifact *datura.Artifact) error {
	switch datura.Peek[string](artifact, "role") {
	case "trade":
		payload, payloadOK := artifact.PayloadQuiet()

		if payloadOK {
			var updates []struct {
				Symbol    string    `json:"symbol"`
				Price     float64   `json:"price"`
				Timestamp time.Time `json:"timestamp"`
			}

			if json.Unmarshal(payload, &updates) == nil {
				for _, update := range updates {
					if update.Symbol == "" || update.Price <= 0 {
						continue
					}

					eventAt := update.Timestamp

					if eventAt.IsZero() {
						eventAt = time.Now()
					}

					signal.Section.ObservePrice(update.Symbol, update.Price, eventAt)
				}
			}
		}
	case "ticker":
		payload, payloadOK := artifact.PayloadQuiet()

		if payloadOK {
			var updates []struct {
				Symbol    string    `json:"symbol"`
				Last      float64   `json:"last"`
				Bid       float64   `json:"bid"`
				Ask       float64   `json:"ask"`
				Timestamp time.Time `json:"timestamp"`
			}

			if json.Unmarshal(payload, &updates) == nil {
				for _, update := range updates {
					if update.Symbol == "" {
						continue
					}

					price := update.Last

					if price <= 0 && update.Bid > 0 && update.Ask > update.Bid {
						price = (update.Bid + update.Ask) / 2
					}

					if price <= 0 {
						continue
					}

					eventAt := update.Timestamp

					if eventAt.IsZero() {
						eventAt = time.Now()
					}

					signal.Section.ObservePrice(update.Symbol, price, eventAt)
				}
			}
		}
	case "measurement":
		if artifact != nil {
			signal.Measure(*artifact)
		}
	}

	return nil
}

func (signal *Signal) Measure(query datura.Artifact) *datura.Artifact {
	scope, _ := query.Scope()

	feature := signal.featureArtifact(scope)

	if feature == nil {
		return nil
	}

	processed := datura.Acquire("leadlag", datura.APPJSON)

	if processed == nil {
		feature.Release()
		return nil
	}

	payload, payloadOK := feature.PayloadQuiet()

	feature.Release()

	if !payloadOK {
		processed.Release()
		return nil
	}

	if processed.WithPayload(payload) == nil {
		processed.Release()
		return nil
	}

	if flipErr := transport.NewFlipFlop(processed, signal.algo); flipErr != nil {
		_ = processed.WithError(flipErr)
	}

	if datura.Peek[int](processed, "classifier.category") <= 0 {
		processed.Release()
		return nil
	}

	if datura.Peek[float64](processed, "classifier.confidence") <= 0 {
		processed.Release()
		return nil
	}

	processed.WithRole("measurement")
	processed.WithScope(scope)

	feed.InsertMeasurement(signal.tree, processed)

	return processed
}

func (signal *Signal) featureArtifact(scope string) *datura.Artifact {
	snapshot := signal.Section.Features(scope)

	if snapshot.Price <= 0 {
		return nil
	}

	if snapshot.IsAnchor && !snapshot.MoveReady {
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

	samples := []float64{
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
	}

	payload, err := json.Marshal(samples)

	if err != nil {
		return nil
	}

	artifact := datura.Acquire("lag-features", datura.Artifact_Type_json)
	artifact.WithRole("features")
	artifact.WithScope(scope)
	artifact.WithPayload(payload)

	return artifact
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
