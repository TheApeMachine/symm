package exhaust

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
)

type Engine struct {
	sample     *algorithm.DecaySample
	decay      *equation.Decay
	classifier *probability.ScoreClassifier
}

func NewEngine() *Engine {
	return &Engine{
		sample: algorithm.NewDecaySample(),
		decay:  equation.NewDecay(),
		classifier: probability.NewScoreClassifier(
			[]string{"mechanical", "thermal", "fragile", "reversal"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryMechanicalCollapse)),
				float64(logic.CategoryIndex(logic.CategoryThermalExhaustion)),
				float64(logic.CategoryIndex(logic.CategoryFragileExpansion)),
				float64(logic.CategoryIndex(logic.CategoryActiveReversal)),
			},
		),
	}
}

func (engine *Engine) MeasureBook(frame *datura.Artifact) *datura.Artifact {
	input, ready, err := engine.sample.MeasureBook(algorithm.BookflowBookInput{
		Symbol: datura.Peek[string](frame, "symbol"),
		Bids:   engine.levels(frame, "bids"),
		Asks:   engine.levels(frame, "asks"),
	})

	return engine.measure(frame, input, ready, err)
}

func (engine *Engine) MeasureTrade(frame *datura.Artifact) *datura.Artifact {
	input, ready, err := engine.sample.MeasureTrade(algorithm.BookflowTradeInput{
		Symbol:   datura.Peek[string](frame, "symbol"),
		Price:    datura.Peek[float64](frame, "price"),
		Quantity: datura.Peek[float64](frame, "qty"),
		Side:     datura.Peek[string](frame, "side"),
	})

	return engine.measure(frame, input, ready, err)
}

func (engine *Engine) measure(
	frame *datura.Artifact,
	input equation.DecayInput,
	ready bool,
	err error,
) *datura.Artifact {
	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	if !ready {
		return nil
	}

	output, err := engine.decay.Measure(input)

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	if output.Strength <= 0 {
		return nil
	}

	frame.MergeOutput("mechanical", output.Mechanical)
	frame.MergeOutput("thermal", output.Thermal)
	frame.MergeOutput("fragile", output.Fragile)
	frame.MergeOutput("reversal", output.Reversal)
	frame.MergeOutput("urgency", output.Urgency)
	frame.MergeOutput("strength", output.Strength)

	result, err := engine.classifier.Classify(map[string]float64{
		"mechanical": output.Mechanical,
		"thermal":    output.Thermal,
		"fragile":    output.Fragile,
		"reversal":   output.Reversal,
		"strength":   output.Strength,
	})

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	for key, value := range result.Outputs() {
		frame.MergeOutput(key, value)
	}

	return completeMeasurement(frame)
}

func (engine *Engine) levels(frame *datura.Artifact, side string) []algorithm.BookLevel {
	levels := make([]algorithm.BookLevel, 0)

	for index := 0; ; index++ {
		price := datura.Peek[float64](frame, side, index, "price")

		if price <= 0 {
			return levels
		}

		levels = append(levels, algorithm.BookLevel{
			Price:    price,
			Quantity: datura.Peek[float64](frame, side, index, "qty"),
		})
	}
}
