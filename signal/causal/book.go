package causal

import (
	"math"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	pearl *algorithm.Pearl
	bids  map[string]map[float64]float64
	asks  map[string]map[float64]float64
	last  map[string]float64
}

func NewBook() *Book {
	return &Book{
		pearl: algorithm.NewPearl(algorithm.PearlConfig{
			Target:          2,
			Treatment:       1,
			Controls:        []int{0},
			CategoryIndexes: []float64{1, 2, 3, 4},
		}),
		bids: map[string]map[float64]float64{},
		asks: map[string]map[float64]float64{},
		last: map[string]float64{},
	}
}

func (book *Book) Measure(
	row kraken.BookData,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	bids, ok := book.bids[row.Symbol]

	if !ok {
		bids = map[float64]float64{}
		book.bids[row.Symbol] = bids
	}

	asks, ok := book.asks[row.Symbol]

	if !ok {
		asks = map[float64]float64{}
		book.asks[row.Symbol] = asks
	}

	for _, level := range row.Bids {
		if level.Price <= 0 {
			continue
		}

		if level.Qty <= 0 {
			delete(bids, level.Price)
			continue
		}

		bids[level.Price] = level.Qty
	}

	for _, level := range row.Asks {
		if level.Price <= 0 {
			continue
		}

		if level.Qty <= 0 {
			delete(asks, level.Price)
			continue
		}

		asks[level.Price] = level.Qty
	}

	bid := 0.0
	bidQty := 0.0

	for price, quantity := range bids {
		if price <= bid {
			continue
		}

		bid = price
		bidQty = quantity
	}

	ask := 0.0
	askQty := 0.0

	for price, quantity := range asks {
		if ask > 0 && price >= ask {
			continue
		}

		ask = price
		askQty = quantity
	}

	if bid <= 0 || ask <= bid {
		return nil, nil
	}

	depth := bidQty + askQty
	liquidity := 0.0
	imbalance := 0.0

	if depth > 0 {
		liquidity = (ask - bid) / depth
		imbalance = (bidQty - askQty) / depth
	}

	mid := (bid + ask) / 2
	velocity := 0.0
	previous := book.last[row.Symbol]

	if previous > 0 {
		velocity = math.Log(mid / previous)
	}

	book.last[row.Symbol] = mid

	output, ready, err := book.pearl.Measure(algorithm.PearlInput{
		Key:          row.Symbol,
		Row:          []float64{liquidity, imbalance, velocity},
		Intervention: imbalance,
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
