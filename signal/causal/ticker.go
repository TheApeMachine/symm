package causal

import (
	"math"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	pearl *algorithm.Pearl
	last  map[string]float64
}

func NewTicker() *Ticker {
	return &Ticker{
		pearl: algorithm.NewPearl(algorithm.PearlConfig{
			Target:          3,
			Treatment:       1,
			Controls:        []int{0, 2},
			CategoryIndexes: []float64{1, 2, 3, 4},
		}),
		last: map[string]float64{},
	}
}

func (ticker *Ticker) Measure(
	row kraken.TickerData,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	if row.Last <= 0 || math.IsNaN(row.Last) || math.IsInf(row.Last, 0) {
		return nil, nil
	}

	depth := row.BidQty + row.AskQty
	liquidity := 0.0
	pressure := 0.0

	if row.Bid > 0 && row.Ask > row.Bid && depth > 0 {
		liquidity = (row.Ask - row.Bid) / depth
		pressure = (row.BidQty - row.AskQty) / depth
	}

	velocity := 0.0
	previous := ticker.last[row.Symbol]

	if previous > 0 {
		velocity = math.Log(row.Last / previous)
	}

	ticker.last[row.Symbol] = row.Last

	output, ready, err := ticker.pearl.Measure(algorithm.PearlInput{
		Key:          row.Symbol,
		Row:          []float64{liquidity, pressure, row.ChangePct / 100, velocity},
		Intervention: pressure,
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
