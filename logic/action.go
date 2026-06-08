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
	Type     ActionType
	Side     trading.Side
	Symbol   string
	Price    float64
	Quantity float64
	Offset   float64
	Fraction float64
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
