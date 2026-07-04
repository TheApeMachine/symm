package depthflow

import (
	"context"
	"iter"
	"strconv"

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
DepthFlow is the "Weight of the Book" perspective, measuring touch-level book
imbalance with trade-pressure confirmation. Multi-level distance weighting is
owned by nomagique's bookflow primitive.

Spoof Trap is currently scored from L2 book shape contradicted by touch pressure.
A faithful spoof read from add/delete order behavior still needs L3 per-order
events; this implementation does not pretend L2 can prove cancel/fill intent.

1. Loaded Imbalance - book weight agrees with trade pressure.
2. Spoof Trap - deep-book shape contradicts touch pressure.
3. Book Thinning - defensive depth disappears relative to the weighted book.
4. Dense Neutrality - balanced thick depth with low pressure.

# Summary of DepthFlow Categories

| Category         | WBI (Weighted Imbalance) | Trade Pressure    | Market "Feel"        |
|:-----------------|:-------------------------|:------------------|:---------------------|
| Loaded Imbalance | High                     | High (Agrees)     | Structural Gravity   |
| Spoof Trap       | High                     | Low (Contradicts) | Manipulated/Fake     |
| Book Thinning    | Rapidly Falling          | Variable          | Exhaustion/Crumbling |
| Dense Neutrality | Balanced                 | Low               | Robust Stability     |
*/

/*
Signal routes book and trade rows into the shared depth-flow pipeline.
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
		algorithm.NewBookflowSample(datura.Acquire("depthflow", datura.APPJSON)),
		equation.NewBookflow(datura.Acquire(
			"depthflow", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": equation.BookflowInputKeys,
		})),
		probability.NewClassifier(datura.Acquire(
			"depthflow", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"loadedScore",
				"spoofScore",
				"thinScore",
				"neutralScore",
			},
			"categoryIndexes": []float64{
				float64(logic.CategoryIndex(logic.CategoryLoadedImbalance)),
				float64(logic.CategoryIndex(logic.CategorySpoofTrap)),
				float64(logic.CategoryIndex(logic.CategoryBookThinning)),
				float64(logic.CategoryIndex(logic.CategoryDenseNeutrality)),
			},
		})),
	)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		book:   NewBook(algo),
		trade:  NewTrade(algo),
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
					"depthflow: row object required",
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
					"depthflow: row symbol required",
					nil,
				)))) {
					return
				}

				continue
			}

			rowArtifact := datura.Acquire(
				"depthflow", datura.APPJSON,
			).WithRole(
				"measurement",
			).WithScope(
				symbol,
			).WithPayload(
				datura.Map[any](row).Marshal(),
			)
			rowArtifact.SetTimestamp(datapoint.Timestamp())

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

func completeMeasurement(frame *datura.Artifact) *datura.Artifact {
	if datura.Peek[float64](frame, "output", "value") > 0 &&
		datura.Peek[float64](frame, "output", "confidence") > 0 &&
		datura.Peek[float64](frame, "output", "entry_baseline") > 0 &&
		datura.Peek[float64](frame, "output", "exit_baseline") > 0 {
		return frame
	}

	loaded := logic.CategoryIndex(logic.CategoryLoadedImbalance)
	spoof := logic.CategoryIndex(logic.CategorySpoofTrap)
	thin := logic.CategoryIndex(logic.CategoryBookThinning)
	neutral := logic.CategoryIndex(logic.CategoryDenseNeutrality)
	baseline := 0.25

	frame.MergeOutputs(map[string]any{
		"loadedScore":         datura.Peek[float64](frame, "output", "loadedScore"),
		"spoofScore":          datura.Peek[float64](frame, "output", "spoofScore"),
		"thinScore":           datura.Peek[float64](frame, "output", "thinScore"),
		"neutralScore":        datura.Peek[float64](frame, "output", "neutralScore"),
		"probabilities":       []float64{baseline, baseline, baseline, baseline},
		"category":            float64(neutral),
		"confidence":          baseline,
		"confidence_baseline": baseline,
		"distribution": map[string]float64{
			strconv.Itoa(loaded):  baseline,
			strconv.Itoa(spoof):   baseline,
			strconv.Itoa(thin):    baseline,
			strconv.Itoa(neutral): baseline,
		},
		"entry_baseline": baseline,
		"exit_baseline":  baseline,
		"strength":       datura.Peek[float64](frame, "output", "strength"),
		"value":          float64(neutral),
	})
	frame.Poke("output", "root")
	frame.Poke([]string{
		"loadedScore",
		"spoofScore",
		"thinScore",
		"neutralScore",
		"probabilities",
		"category",
		"confidence",
		"confidence_baseline",
		"distribution",
		"entry_baseline",
		"exit_baseline",
		"strength",
		"value",
	}, "inputs")

	return frame
}
