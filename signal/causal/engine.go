package causal

import (
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
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

func (engine *Engine) MeasureTicker(frame *datura.Artifact) *datura.Artifact {
	output, ready, err := engine.pearl.MeasureTicker(algorithm.PearlTickerInput{
		Symbol:    datura.Peek[string](frame, "symbol"),
		Last:      datura.Peek[float64](frame, "last"),
		ChangePct: datura.Peek[float64](frame, "change_pct"),
		Bid:       datura.Peek[float64](frame, "bid"),
		Ask:       datura.Peek[float64](frame, "ask"),
		BidQty:    datura.Peek[float64](frame, "bid_qty"),
		AskQty:    datura.Peek[float64](frame, "ask_qty"),
	})

	return engine.measure(frame, output, ready, err)
}

func (engine *Engine) MeasureBook(frame *datura.Artifact) *datura.Artifact {
	output, ready, err := engine.pearl.MeasureBook(algorithm.PearlBookInput{
		Symbol: datura.Peek[string](frame, "symbol"),
		Bids:   engine.levels(frame, "bids"),
		Asks:   engine.levels(frame, "asks"),
	})

	return engine.measure(frame, output, ready, err)
}

func (engine *Engine) MeasureTrade(frame *datura.Artifact) *datura.Artifact {
	output, ready, err := engine.pearl.MeasureTrade(algorithm.PearlTradeInput{
		Symbol:   datura.Peek[string](frame, "symbol"),
		Price:    datura.Peek[float64](frame, "price"),
		Quantity: datura.Peek[float64](frame, "qty"),
		Side:     datura.Peek[string](frame, "side"),
	})

	return engine.measure(frame, output, ready, err)
}

func (engine *Engine) measure(
	frame *datura.Artifact,
	output algorithm.PearlOutput,
	ready bool,
	err error,
) *datura.Artifact {
	if err != nil {
		return frame.WithError(errnie.Error(errnie.Err(errnie.UnprocessableContent, err.Error(), err)))
	}

	if !ready {
		return nil
	}

	frame.MergeOutputs(output.Outputs())
	frame.Poke("output", "root")
	errnie.Error(frame.SetOrigin(string(logic.SourceCausal)))
	markCounterfactual(frame)

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
