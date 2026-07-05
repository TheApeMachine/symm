package causal

import (
	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Trade struct {
	pearl *algorithm.Pearl
}

func NewTrade() *Trade {
	return &Trade{
		pearl: algorithm.NewPearl(algorithm.PearlConfig{}),
	}
}

func (trade *Trade) Measure(
	row kraken.TradeData,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	output, ready, err := trade.pearl.MeasureTrade(algorithm.PearlTradeInput{
		Symbol:   row.Symbol,
		Price:    row.Price,
		Quantity: row.Qty,
		Side:     row.Side,
	})
	if err != nil || !ready {
		return nil, err
	}

	return []*types.Measurement{&types.Measurement{}}, nil
}
