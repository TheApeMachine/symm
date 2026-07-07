package toxicity

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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
				float64(types.CategoryIndex(types.CategoryToxicBluff)),
				float64(types.CategoryIndex(types.CategoryLiquidityVacuum)),
				float64(types.CategoryIndex(types.CategoryHardSupport)),
			},
		),
	}
}

func (engine *Engine) MeasureLevel3(row kraken.Level3Data) (*types.Measurement, error) {
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
		measurement.Metrics["l3"] = 1
	}

	return measurement, nil
}

func (engine *Engine) MeasureTrade(row kraken.TradeData) (*types.Measurement, error) {
	input, ready, err := engine.sample.MeasureTrade(algorithm.BookflowTradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price.Float64(),
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
) (*types.Measurement, error) {
	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if !ready {
		return nil, nil
	}

	if input.LastPrice <= 0 {
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

	categories := []types.CategoryType{
		types.ToxicBluff,
		types.LiquidityVacuum,
		types.HardSupport,
	}
	strengths := []float64{
		output.BluffScore,
		output.VacuumScore,
		output.SupportScore,
	}
	categoryRows := make([]types.Category, 0, len(categories))

	for index, category := range categories {
		confidence := 0.0

		if index < len(result.Probabilities) {
			confidence = result.Probabilities[index]
		}

		categoryRows = append(categoryRows, types.Category{
			Type:       category,
			Confidence: confidence,
			Strength:   strengths[index],
		})
	}

	measurement := &types.Measurement{
		Source:        types.SourceToxicity,
		Symbol:        symbol,
		At:            at,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"bluffScore":   output.BluffScore,
			"vacuumScore":  output.VacuumScore,
			"supportScore": output.SupportScore,
			"strength":     output.Strength,
		},
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
