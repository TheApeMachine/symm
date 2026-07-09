package exhaust

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/nomagique/algorithm/book/flow"
	"github.com/theapemachine/nomagique/equation"
	"github.com/theapemachine/nomagique/probability"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
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
				float64(types.CategoryIndex(types.CategoryMechanicalCollapse)),
				float64(types.CategoryIndex(types.CategoryThermalExhaustion)),
				float64(types.CategoryIndex(types.CategoryFragileExpansion)),
				float64(types.CategoryIndex(types.CategoryActiveReversal)),
			},
		),
	}
}

func (engine *Engine) MeasureBook(row kraken.BookData) (*types.Measurement, error) {
	bids, err := engine.levels(row.Bids, row.PriceIncrement)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	asks, err := engine.levels(row.Asks, row.PriceIncrement)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	input, ready, err := engine.sample.MeasureBook(flow.BookInput{
		Symbol:   row.Symbol,
		TickSize: row.PriceIncrement.Float64(),
		Bids:     bids,
		Asks:     asks,
	})

	return engine.measure("book", row.Symbol, row.Timestamp, input, ready, err)
}

func (engine *Engine) MeasureTrade(row kraken.TradeData) (*types.Measurement, error) {
	input, ready, err := engine.sample.MeasureTrade(flow.TradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price.Float64(),
		Quantity: row.Qty,
		Side:     row.Side,
	})

	return engine.measure("trades", row.Symbol, row.Timestamp, input, ready, err)
}

func (engine *Engine) measure(
	stream string,
	symbol string,
	at time.Time,
	input equation.DecayInput,
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

	output, err := engine.decay.Measure(input)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	if output.Strength <= 0 {
		return nil, nil
	}

	result, err := engine.classifier.Classify(map[string]float64{
		"mechanical": output.Mechanical,
		"thermal":    output.Thermal,
		"fragile":    output.Fragile,
		"reversal":   output.Reversal,
		"strength":   output.Strength,
	})

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.UnprocessableContent, err.Error(), err,
		))
	}

	categories := []types.CategoryType{
		types.MechanicalCollapse,
		types.ThermalExhaustion,
		types.FragileExpansion,
		types.ActiveReversal,
	}
	strengths := []float64{
		output.Mechanical,
		output.Thermal,
		output.Fragile,
		output.Reversal,
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
		Source:        types.SourceExhaustion,
		Stream:        stream,
		Symbol:        symbol,
		At:            at,
		EntryBaseline: result.EntryBaseline,
		ExitBaseline:  result.ExitBaseline,
		Categories:    categoryRows,
		Metrics: map[string]float64{
			"mechanical": output.Mechanical,
			"thermal":    output.Thermal,
			"fragile":    output.Fragile,
			"reversal":   output.Reversal,
			"urgency":    output.Urgency,
			"strength":   output.Strength,
			"value":      output.Value,
			"category":   output.Category,
		},
	}

	return measurement, nil
}

func (engine *Engine) levels(
	rows []kraken.BookLevel,
	increment decimal.Decimal,
) ([]flow.BookLevel, error) {
	levels := make([]flow.BookLevel, 0, len(rows))

	for _, row := range rows {
		tick, err := kraken.PriceTick(row.Price, increment)

		if err != nil {
			return nil, err
		}

		levels = append(levels, flow.BookLevel{
			Price:    row.Price.Float64(),
			Ticks:    tick,
			Quantity: row.Qty,
		})
	}

	return levels, nil
}
