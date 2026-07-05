package causal

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Trade struct {
	pearl *algorithm.Pearl
	last  map[string]float64
}

func NewTrade() *Trade {
	return &Trade{
		pearl: algorithm.NewPearl(algorithm.PearlConfig{
			Target:          1,
			Treatment:       0,
			CategoryIndexes: []float64{1, 2, 3, 4},
		}),
		last: map[string]float64{},
	}
}

func (trade *Trade) Measure(
	row kraken.TradeData,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	if row.Symbol == "" || row.Price <= 0 || row.Qty <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal trade: symbol, price, and quantity required",
			nil,
		))
	}

	flow := row.Qty

	if row.Side == "sell" {
		flow = -row.Qty
	}

	if row.Side != "buy" && row.Side != "sell" {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"causal trade: side must be buy or sell",
			nil,
		))
	}

	velocity := 0.0
	previous := trade.last[row.Symbol]

	if previous > 0 {
		velocity = math.Log(row.Price / previous)
	}

	trade.last[row.Symbol] = row.Price

	output, ready, err := trade.pearl.Measure(algorithm.PearlInput{
		Key:          row.Symbol,
		Row:          []float64{flow, velocity},
		Intervention: flow,
	})

	if err != nil || !ready {
		return nil, err
	}

	probabilities := output.Probabilities

	categories := []types.CategoryType{
		types.EndogenousAlpha,
		types.SystemicBeta,
		types.LiquidityShock,
		types.CausalNoise,
	}

	scores := []float64{
		output.UpliftScore,
		output.AssociationScore,
		math.Abs(output.Condition) + math.Abs(output.Contagion),
		output.Residual(),
	}

	measurement := &types.Measurement{
		Categories: make([]types.Category, 0, len(categories)),
	}

	for index, category := range categories {
		confidence := 0.0

		if index < len(probabilities) {
			confidence = probabilities[index]
		}

		measurement.Categories = append(measurement.Categories, types.Category{
			Type:       category,
			Confidence: confidence,
			Strength:   scores[index],
		})
	}

	return []*types.Measurement{measurement}, nil
}
