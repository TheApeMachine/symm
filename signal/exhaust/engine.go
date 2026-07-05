package exhaust

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

func (engine *Engine) MeasureBook(row kraken.BookData) (*logic.Measurement, error) {
	input, ready, err := engine.sample.MeasureBook(algorithm.BookflowBookInput{
		Symbol: row.Symbol,
		Bids:   engine.levels(row.Bids),
		Asks:   engine.levels(row.Asks),
	})

	return engine.measure(row.Symbol, row.Timestamp, input, ready, err)
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
	input equation.DecayInput,
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

	measurement := logic.NewMeasurement(logic.SourceExhaustion, symbol, at)
	measurement.AddMetric("mechanical", output.Mechanical)
	measurement.AddMetric("thermal", output.Thermal)
	measurement.AddMetric("fragile", output.Fragile)
	measurement.AddMetric("reversal", output.Reversal)
	measurement.AddMetric("urgency", output.Urgency)
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

func (engine *Engine) levels(rows []kraken.BookLevel) []algorithm.BookLevel {
	levels := make([]algorithm.BookLevel, 0, len(rows))

	for _, row := range rows {
		levels = append(levels, algorithm.BookLevel{
			Price:    row.Price,
			Quantity: row.Qty,
		})
	}

	return levels
}
