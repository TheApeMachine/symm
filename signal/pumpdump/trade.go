package pumpdump

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Trade struct {
	clock      *structure.ClockRing[*datura.Artifact]
	sample     *algorithm.TradeFlowSample
	flow       *equation.Flow
	classifier *probability.ScoreClassifier
}

func NewTrade() *Trade {
	return &Trade{
		clock:  structure.NewClockRing[*datura.Artifact](1, 1, 1),
		sample: algorithm.NewTradeFlowSample(),
		flow:   equation.NewFlow(),
		classifier: probability.NewScoreClassifier(
			[]string{"absorption", "drive", "balance", "starvation"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryHiddenAbsorption)),
				float64(logic.CategoryIndex(logic.CategoryAggressiveDrive)),
				float64(logic.CategoryIndex(logic.CategoryStochasticBalance)),
				float64(logic.CategoryIndex(logic.CategoryVolumeStarvation)),
			},
		),
	}
}

func (trade *Trade) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	input, ready, err := trade.sample.Measure(algorithm.TradeFlowInput{
		Symbol:   datura.Peek[string](frame, "symbol"),
		Price:    datura.Peek[float64](frame, "price"),
		Quantity: datura.Peek[float64](frame, "qty"),
		Side:     datura.Peek[string](frame, "side"),
	})

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	if !ready {
		return nil
	}

	output, err := trade.flow.Measure(input)

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		)))
	}

	if output.Value <= 0 {
		return nil
	}

	frame.MergeOutput("absorption", output.Absorption)
	frame.MergeOutput("drive", output.Drive)
	frame.MergeOutput("balance", output.Balance)
	frame.MergeOutput("starvation", output.Starvation)
	frame.MergeOutput("strength", output.Value)
	trade.classify(frame, output)

	evidence := datura.Peek[float64](frame, "output", "absorption") +
		datura.Peek[float64](frame, "output", "drive") +
		datura.Peek[float64](frame, "output", "balance") +
		datura.Peek[float64](frame, "output", "starvation")

	if evidence <= 0 {
		return nil
	}

	frame.Poke("output", "root")
	frame.Poke([]string{
		"probabilities",
		"category",
		"confidence",
		"confidence_baseline",
		"distribution",
		"entry_baseline",
		"exit_baseline",
		"strength",
		"absorption",
		"drive",
		"balance",
		"starvation",
	}, "inputs")

	return frame
}

func (trade *Trade) classify(frame *datura.Artifact, output equation.FlowOutput) {
	result, err := trade.classifier.Classify(map[string]float64{
		"absorption": output.Absorption,
		"drive":      output.Drive,
		"balance":    output.Balance,
		"starvation": output.Starvation,
		"strength":   output.Value,
	})

	if err != nil {
		frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
		return
	}

	for key, value := range result.Outputs() {
		frame.MergeOutput(key, value)
	}
}
