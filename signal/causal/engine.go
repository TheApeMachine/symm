package causal

import (
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic"
)

type Engine struct {
	pearl *algorithm.Pearl
}

func NewEngine() *Engine {
	return &Engine{
		pearl: algorithm.NewPearl(algorithm.PearlConfig{
			MinHistory: 5,
			CategoryIndexes: []float64{
				float64(logic.CategoryIndex(logic.CategoryEndogenousAlpha)),
				float64(logic.CategoryIndex(logic.CategorySystemicBeta)),
				float64(logic.CategoryIndex(logic.CategoryLiquidityShock)),
				float64(logic.CategoryIndex(logic.CategoryCausalNoise)),
			},
		}),
	}
}

func (engine *Engine) MeasureTicker(row kraken.TickerData) (*logic.Measurement, error) {
	output, ready, err := engine.pearl.MeasureTicker(algorithm.PearlTickerInput{
		Symbol:    row.Symbol,
		Last:      row.Last,
		ChangePct: row.ChangePct,
		Bid:       row.Bid,
		Ask:       row.Ask,
		BidQty:    row.BidQty,
		AskQty:    row.AskQty,
	})

	return engine.measure(row.Symbol, row.Timestamp, output, ready, err)
}

func (engine *Engine) MeasureBook(row kraken.BookData) (*logic.Measurement, error) {
	output, ready, err := engine.pearl.MeasureBook(algorithm.PearlBookInput{
		Symbol: row.Symbol,
		Bids:   engine.levels(row.Bids),
		Asks:   engine.levels(row.Asks),
	})

	return engine.measure(row.Symbol, row.Timestamp, output, ready, err)
}

func (engine *Engine) MeasureTrade(row kraken.TradeData) (*logic.Measurement, error) {
	output, ready, err := engine.pearl.MeasureTrade(algorithm.PearlTradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price,
		Quantity: row.Qty,
		Side:     row.Side,
	})

	return engine.measure(row.Symbol, row.Timestamp, output, ready, err)
}

func (engine *Engine) measure(
	symbol string,
	at time.Time,
	output algorithm.PearlOutput,
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

	measurement := logic.NewMeasurement(logic.SourceCausal, symbol, at)
	measurement.AddMetric("alphaScore", output.AlphaScore)
	measurement.AddMetric("betaScore", output.BetaScore)
	measurement.AddMetric("shockScore", output.ShockScore)
	measurement.AddMetric("noiseScore", output.NoiseScore)
	measurement.AddMetric("association", output.Association)
	measurement.AddMetric("intervention", output.Intervention)
	measurement.AddMetric("interventionScore", output.InterventionScore)
	measurement.AddMetric("uplift", output.Uplift)
	measurement.AddMetric("upliftScore", output.UpliftScore)
	measurement.AddMetric("contagion", output.Contagion)
	measurement.AddMetric("condition", output.Condition)
	measurement.AddMetric("strength", output.Strength)

	if output.Inverted {
		measurement.AddMetric("inverted", 1)
	}

	if output.InterventionScore > 0 && output.UpliftScore > 0 {
		measurement.CounterfactualReady = true
	}

	if err := measurement.ApplyClassifier(
		output.Value,
		output.Confidence,
		output.EntryBaseline,
		output.ExitBaseline,
		output.Strength,
		output.Distribution,
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
