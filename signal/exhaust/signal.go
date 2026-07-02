package exhaust

import (
	"context"
	"iter"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/dmt"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

/*
Exhaust is the "Exit Thesis" perspective, tracking microstructure decay to
advise on the urgency of closing an open position.

1. What it measures exactly (in isolation)

The Exhaust signal tracks microstructure decay to advise on the urgency of
closing an open position. Unlike entry signals that look for momentum ignition,
Exhaust looks for momentum rot.

Book Thinning: Measures the trend of bid/ask depth; if depth is disappearing
as price moves, the move is "hollow".

Pressure Fade: Tracks the decay in trade pressure; it signals when the
aggressive "hitters" have run out of ammunition.

Spread Widening: Monitors the bid/ask spread; widening spreads during a trend
indicate increasing mechanical resistance and risk.

Imbalance Flip: Detects when the "weight" of the book flips from the support
side to the resistance side.

---

2. Semantically, what story does it tell?

The Exhaust signal tells the story of when to leave: momentum rot and thesis
decay.

1. Mechanical Collapse - book thinning dominates.
2. Thermal Exhaustion - trade pressure fade dominates.
3. Fragile Expansion - spread widening dominates.
4. Active Reversal - book imbalance flip dominates.

# Summary of Exhaust Categories

| Category            | Primary Metric  | Urgency  | Market "Feel"                    |
|:--------------------|:----------------|:---------|:---------------------------------|
| Mechanical Collapse | Book Thinning   | High     | Crumbling Walls / Flash-Risk     |
| Thermal Exhaustion  | Pressure Fade   | Moderate | Dying Momentum / Topping Out     |
| Fragile Expansion   | Spread Widen    | Moderate | Increasing Friction / Risky Hold |
| Active Reversal     | Imbalance Flip  | High     | Sentiment Flip / Counter-Attack  |

Current implementation consumes book and trade artifacts and uses nomagique's
decay primitive. L3 can improve per-order delete/fill attribution, but this
signal does not claim order-event truth from L2.
*/

/*
Signal routes book and trade rows into the shared exhaust decay pipeline.
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
	book   *Book
	trade  *Trade
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	algo := nomagique.Number(
		algorithm.NewDecaySample(datura.Acquire("exhaust", datura.APPJSON)),
		equation.NewDecay(datura.Acquire(
			"exhaust", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": equation.DecayInputKeys,
		})),
	)
	classifier := probability.NewClassifier(datura.Acquire(
		"exhaust", datura.APPJSON,
	).WithAttributes(datura.Map[any]{
		"inputs": []string{
			"mechanical",
			"thermal",
			"fragile",
			"reversal",
		},
		"categoryIndexes": []float64{
			float64(logic.CategoryIndex(logic.CategoryMechanicalCollapse)),
			float64(logic.CategoryIndex(logic.CategoryThermalExhaustion)),
			float64(logic.CategoryIndex(logic.CategoryFragileExpansion)),
			float64(logic.CategoryIndex(logic.CategoryActiveReversal)),
		},
	}))

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		book:   NewBook(algo, classifier),
		trade:  NewTrade(algo, classifier),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"book", "trade"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		role := datura.Peek[string](datapoint, "role")

		if role != "book" && role != "trade" {
			return
		}

		data := datura.Peek[[]any](datapoint, "data")

		for _, item := range data {
			row, ok := item.(map[string]any)

			if !ok {
				if !yield(datapoint.WithError(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"exhaust: row object required",
					nil,
				)))) {
					return
				}

				continue
			}

			symbol, ok := row["symbol"].(string)

			if !ok {
				if !yield(datapoint.WithError(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"exhaust: row symbol required",
					nil,
				)))) {
					return
				}

				continue
			}

			rowArtifact := datura.Acquire(
				"exhaust", datura.APPJSON,
			).WithRole(
				"measurement",
			).WithScope(
				symbol,
			).WithPayload(
				datura.Map[any](row).Marshal(),
			)
			rowArtifact.SetTimestamp(datapoint.Timestamp())
			errnie.Error(rowArtifact.SetOrigin(string(logic.SourceExhaustion)))

			switch role {
			case "book":
				if !yield(signal.book.Measure(rowArtifact, crossSection)) {
					return
				}
			case "trade":
				if !yield(signal.trade.Measure(rowArtifact, crossSection)) {
					return
				}
			}
		}
	}
}

func (signal *Signal) Error() error {
	return signal.err
}

func (signal *Signal) Close() error {
	signal.cancel()

	return nil
}
