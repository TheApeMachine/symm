package logic

import "github.com/theapemachine/symm/kraken/trading"

type ActionType uint8

const (
	ActionNone ActionType = iota
	ActionLimit
	ActionMarket
	ActionIceberg
	ActionStopLoss
	ActionStopLossLimit
	ActionTakeProfit
	ActionTakeProfitLimit
	ActionTrailingStop
	ActionTrailingStopLimit
	ActionSettlePosition
)

type Action struct {
	Type     ActionType   `yaml:"type"`
	Side     trading.Side `yaml:"side"`
	Symbol   string       `yaml:"symbol"`
	Price    float64      `yaml:"price"`
	Quantity float64      `yaml:"quantity"`
	Offset   float64      `yaml:"offset"`
	Fraction float64      `yaml:"fraction"`
}

func NewAction(
	actionType ActionType,
	side trading.Side,
	symbol string,
	price float64,
	quantity float64,
	offset float64,
	fraction float64,
	strategy string,
) *Action {
	return &Action{
		Type:     actionType,
		Side:     side,
		Symbol:   symbol,
		Price:    price,
		Quantity: quantity,
		Offset:   offset,
		Fraction: fraction,
	}
}
