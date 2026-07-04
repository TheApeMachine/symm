package pumpdump

import (
	"io"
	"strconv"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Trade struct {
	clock *structure.ClockRing[*datura.Artifact]
	algo  io.ReadWriteCloser
}

func NewTrade() *Trade {
	trade := &Trade{
		clock: structure.NewClockRing[*datura.Artifact](1, 1, 1),
	}

	trade.algo = nomagique.Number(
		algorithm.NewTradeFlowSample(datura.Acquire("pumpdump", datura.APPJSON)),
		equation.NewFlow(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": equation.FlowInputKeys,
		})),
		probability.NewClassifier(datura.Acquire(
			"pumpdump", datura.APPJSON,
		).WithAttributes(datura.Map[any]{
			"inputs": []string{
				"absorption",
				"drive",
				"balance",
				"starvation",
			},
			"categoryIndexes": []float64{
				float64(logic.CategoryIndex(logic.CategoryHiddenAbsorption)),
				float64(logic.CategoryIndex(logic.CategoryAggressiveDrive)),
				float64(logic.CategoryIndex(logic.CategoryStochasticBalance)),
				float64(logic.CategoryIndex(logic.CategoryVolumeStarvation)),
			},
		})),
	)

	return trade
}

func (trade *Trade) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	if err := nomagique.RoundTripArtifact(frame, trade.algo); err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	return completeTradeMeasurement(frame)
}

func completeTradeMeasurement(frame *datura.Artifact) *datura.Artifact {
	if datura.Peek[float64](frame, "output", "value") > 0 &&
		datura.Peek[float64](frame, "output", "confidence") > 0 &&
		datura.Peek[float64](frame, "output", "entry_baseline") > 0 &&
		datura.Peek[float64](frame, "output", "exit_baseline") > 0 {
		return frame
	}

	absorption := logic.CategoryIndex(logic.CategoryHiddenAbsorption)
	drive := logic.CategoryIndex(logic.CategoryAggressiveDrive)
	balance := logic.CategoryIndex(logic.CategoryStochasticBalance)
	starvation := logic.CategoryIndex(logic.CategoryVolumeStarvation)
	baseline := 0.25

	frame.MergeOutputs(map[string]any{
		"absorption":          datura.Peek[float64](frame, "output", "absorption"),
		"drive":               datura.Peek[float64](frame, "output", "drive"),
		"balance":             datura.Peek[float64](frame, "output", "balance"),
		"starvation":          datura.Peek[float64](frame, "output", "starvation"),
		"probabilities":       []float64{baseline, baseline, baseline, baseline},
		"category":            float64(balance),
		"confidence":          baseline,
		"confidence_baseline": baseline,
		"distribution": map[string]float64{
			strconv.Itoa(absorption): baseline,
			strconv.Itoa(drive):      baseline,
			strconv.Itoa(balance):    baseline,
			strconv.Itoa(starvation): baseline,
		},
		"entry_baseline": baseline,
		"exit_baseline":  baseline,
		"strength":       datura.Peek[float64](frame, "output", "strength"),
		"value":          float64(balance),
	})
	frame.Poke("output", "root")
	frame.Poke([]string{
		"absorption",
		"drive",
		"balance",
		"starvation",
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
