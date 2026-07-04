package hawkes

import (
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
	"github.com/theapemachine/symm/market"
)

type Trade struct {
	clock      *structure.ClockRing[*datura.Artifact]
	sample     *algorithm.TradeExcitationSample
	excitation *algorithm.Excitation
	classifier *probability.ScoreClassifier
}

func NewTrade() *Trade {
	return &Trade{
		clock:      structure.NewClockRing[*datura.Artifact](1, 1, 1),
		sample:     algorithm.NewTradeExcitationSample(),
		excitation: algorithm.NewExcitation(),
		classifier: probability.NewScoreClassifier(
			[]string{"frenzy", "saturation", "organic", "exhaustion"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryFrenzy)),
				float64(logic.CategoryIndex(logic.CategorySaturation)),
				float64(logic.CategoryIndex(logic.CategoryOrganic)),
				float64(logic.CategoryIndex(logic.CategoryExhaustion)),
			},
		),
	}
}

func (trade *Trade) Measure(
	frame *datura.Artifact,
	crossSection *market.CrossSection,
) *datura.Artifact {
	if observed := datura.Peek[string](frame, "timestamp"); observed != "" {
		stamp, err := time.Parse(time.RFC3339Nano, observed)

		if err != nil {
			return frame.WithError(errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				err.Error(),
				err,
			)))
		}

		frame.SetTimestamp(stamp.UnixNano())
	}

	input, ready, err := trade.sample.MeasureTrade(algorithm.TradeExcitationInput{
		Symbol:   datura.Peek[string](frame, "symbol"),
		Side:     datura.Peek[string](frame, "side"),
		UnixNano: frame.Timestamp(),
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

	output, ready, err := trade.excitation.Measure(input)

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	if !ready || output.Strength <= 0 {
		return nil
	}

	frame.MergeOutput("frenzy", output.Frenzy)
	frame.MergeOutput("saturation", output.Saturation)
	frame.MergeOutput("organic", output.Organic)
	frame.MergeOutput("exhaustion", output.Exhaustion)
	frame.MergeOutput("strength", output.Strength)
	frame.MergeOutput("branchingRatio", output.BranchingRatio)
	frame.MergeOutput("spectralRadius", output.SpectralRadius)
	frame.MergeOutput("stationarityMargin", output.StationarityMargin)
	frame.MergeOutput("baselineMu", output.BaselineMu)
	frame.MergeOutput("intensityRatio", output.IntensityRatio)
	trade.classify(frame, output)

	return completeMeasurement(frame)
}

func (trade *Trade) classify(frame *datura.Artifact, output algorithm.ExcitationOutcome) {
	result, err := trade.classifier.Classify(map[string]float64{
		"frenzy":     output.Frenzy,
		"saturation": output.Saturation,
		"organic":    output.Organic,
		"exhaustion": output.Exhaustion,
		"strength":   output.Strength,
	})

	if err != nil {
		frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
		return
	}

	for key, value := range result.Outputs() {
		frame.MergeOutput(key, value)
	}
}

func completeMeasurement(frame *datura.Artifact) *datura.Artifact {
	evidence := datura.Peek[float64](frame, "output", "frenzy") +
		datura.Peek[float64](frame, "output", "saturation") +
		datura.Peek[float64](frame, "output", "organic") +
		datura.Peek[float64](frame, "output", "exhaustion")

	if evidence <= 0 {
		return nil
	}

	if datura.Peek[float64](frame, "output", "value") > 0 &&
		datura.Peek[float64](frame, "output", "confidence") > 0 &&
		datura.Peek[float64](frame, "output", "entry_baseline") > 0 &&
		datura.Peek[float64](frame, "output", "exit_baseline") > 0 {
		frame.Poke("output", "root")
		return frame
	}

	return nil
}
