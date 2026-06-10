package logic

import (
	"fmt"

	"github.com/theapemachine/symm/kraken/trading"
)

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
	Type      ActionType   `yaml:"type"`
	Side      trading.Side `yaml:"side"`
	Symbol    string       `yaml:"symbol"`
	Price     float64      `yaml:"price"`
	Quantity  float64      `yaml:"quantity"`
	Offset    float64      `yaml:"offset"`
	Fraction  float64      `yaml:"fraction"`
	BranchKey string       `yaml:"-"`
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

func (actionType ActionType) IsExit() bool {
	switch actionType {
	case ActionStopLoss, ActionStopLossLimit,
		ActionTakeProfit, ActionTakeProfitLimit,
		ActionTrailingStop, ActionTrailingStopLimit,
		ActionSettlePosition:
		return true
	default:
		return false
	}
}

func (actionType ActionType) KrakenOrderType() (trading.OrderType, error) {
	switch actionType {
	case ActionLimit:
		return trading.Limit, nil
	case ActionMarket, ActionSettlePosition:
		return trading.Market, nil
	case ActionIceberg:
		return trading.Iceberg, nil
	case ActionStopLoss:
		return trading.StopLoss, nil
	case ActionStopLossLimit:
		return trading.StopLossLimit, nil
	case ActionTakeProfit:
		return trading.TakeProfit, nil
	case ActionTakeProfitLimit:
		return trading.TakeProfitLimit, nil
	case ActionTrailingStop:
		return trading.TrailingStop, nil
	case ActionTrailingStopLimit:
		return trading.TrailingStopLimit, nil
	default:
		return "", fmt.Errorf("logic: unsupported order action %q", actionType.String())
	}
}

func (actionType ActionType) String() string {
	switch actionType {
	case ActionLimit:
		return "limit"
	case ActionMarket:
		return "market"
	case ActionIceberg:
		return "iceberg"
	case ActionStopLoss:
		return "stop_loss"
	case ActionStopLossLimit:
		return "stop_loss_limit"
	case ActionTakeProfit:
		return "take_profit"
	case ActionTakeProfitLimit:
		return "take_profit_limit"
	case ActionTrailingStop:
		return "trailing_stop"
	case ActionTrailingStopLimit:
		return "trailing_stop_limit"
	case ActionSettlePosition:
		return "settle_position"
	default:
		return ""
	}
}
