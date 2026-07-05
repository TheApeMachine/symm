package toxicity

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
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

func (engine *Engine) MeasureLevel3(row kraken.Level3Data) (*logic.Measurement, error) {
	input, ready, err := engine.sample.MeasureLevel3(algorithm.BookQualityLevel3Input{
		Symbol: row.Symbol,
		Bids:   engine.orders(row.Bids),
		Asks:   engine.orders(row.Asks),
	})

	measurement, err := engine.measure(row.Symbol, row.Timestamp, input, ready, err)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if measurement != nil {
		measurement.AddMetric("l3", 1)
	}

	return measurement, nil
}

func (engine *Engine) MeasureTrade(row kraken.TradeData) (*logic.Measurement, error) {
	input, ready, err := engine.sample.MeasureTrade(algorithm.BookflowTradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price,
		Quantity: row.Qty,
		Side:     row.Side,
	})

	return engine.measure(row.Symbol, row.Timestamp, input, ready, err)
}

func (engine *Engine) measure(
	symbol string,
	at time.Time,
	input equation.BookQualityInput,
	ready bool,
	err error,
) (*logic.Measurement, error) {
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !ready {
		return nil, nil
	}

	output, err := engine.bookQuality.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if output.Strength <= 0 {
		return nil, nil
	}

	result, err := engine.classifier.Classify(map[string]float64{
		"bluffScore":   output.BluffScore,
		"vacuumScore":  output.VacuumScore,
		"supportScore": output.SupportScore,
		"strength":     output.Strength,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	measurement := logic.NewMeasurement(logic.SourceToxicity, symbol, at)
	measurement.AddMetric("bluffScore", output.BluffScore)
	measurement.AddMetric("vacuumScore", output.VacuumScore)
	measurement.AddMetric("supportScore", output.SupportScore)
	measurement.AddMetric("strength", output.Strength)

	if err := measurement.ApplyClassifier(
		result.Value,
		result.Confidence,
		result.EntryBaseline,
		result.ExitBaseline,
		result.Strength,
		result.Distribution,
	); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if err := measurement.Ready(); err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	return measurement, nil
}

func (engine *Engine) orders(rows []kraken.Level3Order) []algorithm.BookQualityOrderEvent {
	orders := make([]algorithm.BookQualityOrderEvent, 0, len(rows))
	for _, row := range rows {
		orders = append(orders, algorithm.BookQualityOrderEvent{
			Event:    row.Event,
			OrderID:  row.OrderID,
			Price:    row.LimitPrice,
			Quantity: row.OrderQty,
		})
	}

	return orders
}
