package toxicity

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/logic"
)

type Engine struct {
	sample      *algorithm.BookQualitySample
	bookQuality *equation.BookQuality
	classifier  *probability.ScoreClassifier
}

func NewEngine() *Engine {
	return &Engine{
		sample:      algorithm.NewBookQualitySample(),
		bookQuality: equation.NewBookQuality(),
		classifier: probability.NewScoreClassifier(
			[]string{"bluffScore", "vacuumScore", "supportScore"},
			[]float64{
				float64(logic.CategoryIndex(logic.CategoryToxicBluff)),
				float64(logic.CategoryIndex(logic.CategoryLiquidityVacuum)),
				float64(logic.CategoryIndex(logic.CategoryHardSupport)),
			},
		),
	}
}

func (engine *Engine) MeasureLevel3(frame *datura.Artifact) *datura.Artifact {
	input, ready, err := engine.sample.MeasureLevel3(algorithm.BookQualityLevel3Input{
		Symbol: datura.Peek[string](frame, "symbol"),
		Bids:   engine.orders(frame, "bids"),
		Asks:   engine.orders(frame, "asks"),
	})

	measurement := engine.measure(frame, input, ready, err)

	if measurement != nil {
		measurement.MergeOutput("l3", 1)
		measurement.Merge("l3", true)
	}

	return measurement
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
	input equation.BookQualityInput,
	ready bool,
	err error,
) *datura.Artifact {
	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	if !ready {
		return nil
	}

	output, err := engine.bookQuality.Measure(input)

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	if output.Strength <= 0 {
		return nil
	}

	frame.MergeOutput("bluffScore", output.BluffScore)
	frame.MergeOutput("vacuumScore", output.VacuumScore)
	frame.MergeOutput("supportScore", output.SupportScore)
	frame.MergeOutput("strength", output.Strength)

	result, err := engine.classifier.Classify(map[string]float64{
		"bluffScore":   output.BluffScore,
		"vacuumScore":  output.VacuumScore,
		"supportScore": output.SupportScore,
		"strength":     output.Strength,
	})

	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	for key, value := range result.Outputs() {
		frame.MergeOutput(key, value)
	}

	return completeMeasurement(frame)
}

func (engine *Engine) orders(frame *datura.Artifact, side string) []algorithm.BookQualityOrderEvent {
	orders := make([]algorithm.BookQualityOrderEvent, 0)

	for index := 0; ; index++ {
		price := datura.Peek[float64](frame, side, index, "limit_price")

		if price <= 0 {
			return orders
		}

		orders = append(orders, algorithm.BookQualityOrderEvent{
			Event:    datura.Peek[string](frame, side, index, "event"),
			OrderID:  datura.Peek[string](frame, side, index, "order_id"),
			Price:    price,
			Quantity: datura.Peek[float64](frame, side, index, "order_qty"),
		})
	}
}
