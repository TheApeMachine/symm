package pumpdump

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Ticker struct {
	clock      *structure.ClockRing[*datura.Artifact]
	ignition   *equation.Ignition
	classifier *probability.ScoreClassifier
}

func NewTicker() *Ticker {
	return &Ticker{
		clock:    structure.NewClockRing[*datura.Artifact](1, 1, 1),
		ignition: equation.NewIgnition(),
		classifier: probability.NewScoreClassifier(
			[]string{"ignition", "compression", "trend", "exhaustion"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryVerticalIgnition)),
				float64(logic.CategoryIndex(logic.CategoryCoiledCompression)),
				float64(logic.CategoryIndex(logic.CategoryOrganicTrend)),
				float64(logic.CategoryIndex(logic.CategoryFadedExhaustion)),
			},
		),
	}
}

func (ticker *Ticker) Measure(
	frame *datura.Artifact, crossSection *market.CrossSection,
) *datura.Artifact {
	output, ready, err := ticker.ignition.Measure(equation.IgnitionInput{
		Symbol: datura.Peek[string](frame, "symbol"),
		Volume: datura.Peek[float64](frame, "volume"),
		Last:   datura.Peek[float64](frame, "last"),
		Bid:    datura.Peek[float64](frame, "bid"),
		Ask:    datura.Peek[float64](frame, "ask"),
	})

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			err.Error(),
			err,
		))
	}

	if !ready {
		return nil
	}

	frame.MergeOutput("rvol", output.RVOL)
	frame.MergeOutput("precursor", output.Precursor)
	frame.MergeOutput("spread", output.Spread)
	frame.MergeOutput("ignition", output.Ignition)
	frame.MergeOutput("compression", output.Compression)
	frame.MergeOutput("trend", output.Trend)
	frame.MergeOutput("exhaustion", output.Exhaustion)
	frame.MergeOutput("strength", output.Strength)
	ticker.classify(frame, output)

	if datura.Peek[float64](frame, "output", "value") > 0 &&
		datura.Peek[float64](frame, "output", "confidence") > 0 &&
		datura.Peek[float64](frame, "output", "entry_baseline") > 0 &&
		datura.Peek[float64](frame, "output", "exit_baseline") > 0 {
		frame.Poke("output", "root")
		frame.Poke([]string{"volume", "last", "bid", "ask"}, "sourceInputs")
		frame.Poke([]string{
			"probabilities",
			"category",
			"confidence",
			"confidence_baseline",
			"distribution",
			"entry_baseline",
			"exit_baseline",
			"strength",
			"ignition",
			"compression",
			"trend",
			"exhaustion",
		}, "inputs")
		return frame
	}

	return nil
}

func (ticker *Ticker) classify(frame *datura.Artifact, output equation.IgnitionOutput) {
	result, err := ticker.classifier.Classify(map[string]float64{
		"ignition":    output.Ignition,
		"compression": output.Compression,
		"trend":       output.Trend,
		"exhaustion":  output.Exhaustion,
		"strength":    output.Strength,
	})

	if err != nil {
		frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
		return
	}

	for key, value := range result.Outputs() {
		frame.MergeOutput(key, value)
	}
}
