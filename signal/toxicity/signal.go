package toxicity

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
Toxicity is the Quality perspective, analyzing the honesty of the book by
tracking per-order liquidity behavior near touch.

Cancel-to-fill asymmetry is an L3 story: level3 add/delete/modify events provide
order identity and price level, while the trade tape marks which deletes
coincided with executed liquidity. L2 book quantity deltas are not used as a
fallback for cancel/fill labels.

1. Liquidity Vacuum

One side is retreating and creating a void.
Indicators: High cancel/fill asymmetry with one side retracting.
Semantic Meaning: Vacuum surcharge - the void itself drives price.

2. Toxic Bluff

Near-touch blocks disappear rather than fill.
Indicators: High cancel/fill ratio at near-touch levels.
Semantic Meaning: Manipulated/fake - a bluff wall about to crumble.

3. Hard Support

Liquidity fills rather than cancels on approach.
Indicators: Low cancel/fill ratio, high fill rate, no side retracting.
Semantic Meaning: Robust/sincere - the wall will hold on contact.

# Summary of Toxicity Categories

| Category         | Cancel/Fill Ratio | Side Retracting | Market "Feel"          |
|:-----------------|:------------------|:----------------|:-----------------------|
| Liquidity Vacuum | High Asymmetry    | One Side        | Vacuum Surcharge       |
| Toxic Bluff      | High              | Near-Touch      | Manipulated / Fake     |
| Hard Support     | Low (High Fill)   | None            | Robust / Sincere       |
*/
type Signal struct {
	ctx    context.Context
	cancel context.CancelFunc
	err    error
	tree   *dmt.Tree
	level3 *Level3
	trade  *Trade
}

func NewSignal(
	ctx context.Context,
	tree *dmt.Tree,
) *Signal {
	ctx, cancel := context.WithCancel(ctx)
	algo := nomagique.Number(
		algorithm.NewBookQualitySample(datura.Acquire(
			"toxicity", datura.APPJSON,
		)),
		equation.NewBookQuality(equation.BookQualityConfig()),
		probability.NewClassifier(datura.Acquire(
			"toxicity", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"bluffScore",
				"vacuumScore",
				"supportScore",
			},
			"scoreRoot": "output",
			"categoryIndexes": []float64{
				float64(logic.CategoryIndex(logic.CategoryToxicBluff)),
				float64(logic.CategoryIndex(logic.CategoryLiquidityVacuum)),
				float64(logic.CategoryIndex(logic.CategoryHardSupport)),
			},
		})),
	)

	return &Signal{
		ctx:    ctx,
		cancel: cancel,
		tree:   tree,
		level3: NewLevel3(algo),
		trade:  NewTrade(algo),
	}
}

func (signal *Signal) IngestRoles() []string {
	return []string{"level3", "trade"}
}

func (signal *Signal) Measure(
	datapoint *datura.Artifact,
	crossSection *market.CrossSection,
) iter.Seq[*datura.Artifact] {
	return func(yield func(*datura.Artifact) bool) {
		role := datura.Peek[string](datapoint, "role")

		if role != "level3" && role != "trade" {
			return
		}

		data := datura.Peek[[]any](datapoint, "data")

		for _, item := range data {
			row, ok := item.(map[string]any)

			if !ok {
				if !yield(datapoint.WithError(errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"toxicity: row object required",
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
					"toxicity: row symbol required",
					nil,
				)))) {
					return
				}

				continue
			}

			rowArtifact := datura.Acquire(
				"toxicity", datura.APPJSON,
			).WithRole(
				"measurement",
			).WithScope(
				symbol,
			).WithPayload(
				datura.Map[any](row).Marshal(),
			)
			rowArtifact.SetTimestamp(datapoint.Timestamp())
			errnie.Error(rowArtifact.SetOrigin(string(logic.SourceToxicity)))

			switch role {
			case "level3":
				if !yield(signal.level3.Measure(rowArtifact, crossSection)) {
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

func (signal *Signal) Close() (err error) {
	err = signal.err
	signal.cancel()

	return err
}

func completeMeasurement(frame *datura.Artifact) *datura.Artifact {
	if datura.Peek[float64](frame, "output", "value") > 0 &&
		datura.Peek[float64](frame, "output", "confidence") > 0 &&
		datura.Peek[float64](frame, "output", "entry_baseline") > 0 &&
		datura.Peek[float64](frame, "output", "exit_baseline") > 0 {
		return frame
	}

	bluff := logic.CategoryIndex(logic.CategoryToxicBluff)
	vacuum := logic.CategoryIndex(logic.CategoryLiquidityVacuum)
	support := logic.CategoryIndex(logic.CategoryHardSupport)
	baseline := 1.0 / 3.0

	frame.MergeOutputs(map[string]any{
		"bluffScore":          datura.Peek[float64](frame, "output", "bluffScore"),
		"vacuumScore":         datura.Peek[float64](frame, "output", "vacuumScore"),
		"supportScore":        datura.Peek[float64](frame, "output", "supportScore"),
		"probabilities":       []float64{baseline, baseline, baseline},
		"category":            float64(support),
		"confidence":          baseline,
		"confidence_baseline": baseline,
		"distribution": map[string]float64{
			strconv.Itoa(bluff):   baseline,
			strconv.Itoa(vacuum):  baseline,
			strconv.Itoa(support): baseline,
		},
		"entry_baseline": baseline,
		"exit_baseline":  baseline,
		"strength":       datura.Peek[float64](frame, "output", "strength"),
		"value":          float64(support),
	})
	frame.Poke("output", "root")
	frame.Poke([]string{
		"bluffScore",
		"vacuumScore",
		"supportScore",
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
