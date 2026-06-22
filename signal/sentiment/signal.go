package sentiment

import (
	"context"
	"io"
	"math"
	"sync"
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/datura/transport"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/qpool"
	marketsection "github.com/theapemachine/symm/market"
)

/*
Sentiment is the Bullish Breadth perspective, measuring global market conviction
by looking at the behavior of the entire universe simultaneously.

1. What it measures exactly (in isolation)

The Sentiment signal measures global market conviction by looking at the behavior
of the entire universe simultaneously.

Market Breadth: The ratio of symbols with a positive changePct versus the total
number of symbols.

Leadership Performance: Tracks the median performance of the top symbols to see
if the leaders are actually leading.

---

2. Semantically, what story does it tell?

The "Rising Tide" Story: It tells you if an asset's move is a solo effort or if
it is being carried by a global risk-on regime where every asset is moving in
unison.

The "Conviction" Story: It distinguishes between a fake leader move (where only
one asset is up) and a high-conviction market environment where breadth exceeds
the dynamically derived majority threshold.

1. Risk-On Surge

Broad participation with strong leadership confirmation.
Indicators: High breadth with strong leader performance.
Semantic Meaning: Rising tide / global buy — macro risk-on regime.

2. Divergent Move

A leader is moving while the broader market stays quiet.
Indicators: Low breadth with strong leader performance.
Semantic Meaning: Idiosyncratic alpha — a local catalyst is at work.

3. Systemic Slump

Breadth and leadership both fail together.
Indicators: Low breadth with weak leader performance.
Semantic Meaning: Global risk-off — no systemic support for new longs.

# Summary of Sentiment Categories

| Category       | Breadth | Leader Strength | Market "Feel"           |
|:---------------|:--------|:----------------|:------------------------|
| Risk-On Surge  | High    | Strong          | Rising Tide / Global Buy|
| Divergent Move | Low     | Strong          | Idiosyncratic Alpha     |
| Systemic Slump | Low     | Weak            | Global Risk-Off         |
*/
/*
Signal measures global market conviction from breadth and leadership performance.
See the struct comment block for category semantics.
*/
type Signal struct {
	ctx          context.Context
	cancel       context.CancelFunc
	err          error
	pool         *qpool.Q[any]
	subscribers  *sync.Map
	algo         io.ReadWriteCloser
	tree         *dmt.Tree
	CrossSection *marketsection.CrossSection
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

	crossSection, err := marketsection.NewCrossSection(&marketsection.CrossSectionConfig{
		MatchWindow: 10 * time.Second,
		ReturnCap:   16,
		MinBars:     4,
		BreadthHist: 16,
	})

	if err != nil {
		cancel()

		return nil
	}

	return &Signal{
		ctx:          ctx,
		cancel:       cancel,
		pool:         pool,
		subscribers:  &sync.Map{},
		tree:         tree,
		CrossSection: crossSection,
		algo: nomagique.Number(
			equation.NewConviction(datura.Acquire("sentiment-conviction", datura.APPJSON)), probability.NewClassifier(
				datura.Acquire("sentiment-classifier", datura.APPJSON).WithAttributes(datura.Map[any]{
					"inputs": []string{"surgeScore", "divergentScore", "slumpScore"},
				}),
			),
		),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"ticker"}
}

func (signal *Signal) Measure(datapoint *datura.Artifact) *datura.Artifact {
	if signal == nil || datapoint == nil || signal.algo == nil || signal.CrossSection == nil {
		return nil
	}

	row, rowErr := marketsection.SymbolFromTicker(datapoint)

	if rowErr != nil {
		return nil
	}

	if errnie.Error(signal.CrossSection.Observe(row)) != nil {
		return nil
	}

	breadth := signal.CrossSection.Breadth(row.Updated)
	signal.CrossSection.RecordBreadth(breadth)

	leaderFlag := 0.0

	if signal.CrossSection.IsLeader(row.Name, row.Value, row.Updated) {
		leaderFlag = 1
	}

	features := []float64{
		breadth,
		row.Value,
		signal.CrossSection.MajorityThreshold(row.Updated),
		leaderFlag,
		row.Value,
	}

	stored := datura.Acquire("sentiment-conviction", datura.APPJSON)
	stored.WithPayload(equation.MarshalFeatureSchema(equation.ConvictionInputKeys, features))

	if errnie.Error(transport.NewFlipFlop(
		stored, signal.algo,
	)) != nil {
		stored.Release()

		return nil
	}

	confidence := datura.Peek[float64](stored, "output", "confidence")
	uniformConfidence := 1.0 / 3.0

	if confidence <= 0 ||
		math.IsNaN(confidence) ||
		math.IsInf(confidence, 0) ||
		confidence <= uniformConfidence+1e-12 {
		stored.Release()

		return nil
	}

	return stored
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}
