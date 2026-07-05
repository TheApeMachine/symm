package causal

import (
	"math"

	"github.com/theapemachine/nomagique/algorithm"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Ticker struct {
	pearl *algorithm.Pearl
}

func NewTicker() *Ticker {
	return &Ticker{
		pearl: algorithm.NewPearl(algorithm.PearlConfig{}),
	}
}

func (ticker *Ticker) Measure(
	row kraken.TickerData,
	_ *types.CrossSection,
) ([]*types.Measurement, error) {
	if row.Last <= 0 || math.IsNaN(row.Last) || math.IsInf(row.Last, 0) {
		return nil, nil
	}

	output, ready, err := ticker.pearl.MeasureTicker(algorithm.PearlTickerInput{
		Symbol:    row.Symbol,
		Last:      row.Last,
		ChangePct: row.ChangePct,
		Bid:       row.Bid,
		Ask:       row.Ask,
		BidQty:    row.BidQty,
		AskQty:    row.AskQty,
	})
	if err != nil || !ready {
		return nil, err
	}

	return []*types.Measurement{&types.Measurement{}}, nil
}
