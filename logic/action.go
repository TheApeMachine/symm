package logic

import (
	"github.com/theapemachine/errnie"
)

type ActionType string

const (
	ActionNone              ActionType = ""
	ActionLimit             ActionType = "limit"
	ActionMarket            ActionType = "market"
	ActionIceberg           ActionType = "iceberg"
	ActionStopLoss          ActionType = "stop_loss"
	ActionStopLossLimit     ActionType = "stop_loss_limit"
	ActionTakeProfit        ActionType = "take_profit"
	ActionTakeProfitLimit   ActionType = "take_profit_limit"
	ActionTrailingStop      ActionType = "trailing_stop"
	ActionTrailingStopLimit ActionType = "trailing_stop_limit"
	ActionSettlePosition    ActionType = "settle_position"
)

type Action struct {
	Type            ActionType   `yaml:"type" json:"type"`
	Side            Side         `yaml:"side" json:"side"`
	Symbol          string       `yaml:"symbol" json:"symbol"`
	DecisionID      string       `yaml:"decision_id,omitempty" json:"decision_id,omitempty"`
	ActionID        string       `yaml:"action_id,omitempty" json:"action_id,omitempty"`
	ClOrdID         string       `yaml:"cl_ord_id,omitempty" json:"cl_ord_id,omitempty"`
	ExchangeOrderID string       `yaml:"exchange_order_id,omitempty" json:"exchange_order_id,omitempty"`
	Price           float64      `yaml:"price" json:"price"`
	Quantity        float64      `yaml:"quantity" json:"quantity"`
	Offset          float64      `yaml:"offset" json:"offset"`
	Fraction        float64      `yaml:"fraction" json:"fraction"`
	EntryConfidence float64      `yaml:"entry_confidence,omitempty" json:"entry_confidence,omitempty"`
	OpportunitySlot bool         `yaml:"opportunity_slot,omitempty" json:"opportunity_slot,omitempty"`
	ReasonSource    SourceType   `yaml:"reason_source,omitempty" json:"reason_source,omitempty"`
	ReasonCategory  CategoryType `yaml:"reason_category,omitempty" json:"reason_category,omitempty"`
}

func NewAction(
	actionType ActionType,
	side Side,
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

func actionForSymbol(action *Action, symbol string) *Action {
	if action == nil {
		return nil
	}

	next := *action
	if next.Symbol == "" {
		next.Symbol = symbol
	}

	return &next
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

func (actionType ActionType) KrakenOrderType() (OrderType, error) {
	switch actionType {
	case ActionLimit:
		return OrderLimit, nil
	case ActionMarket:
		return OrderMarket, nil
	case ActionSettlePosition:
		return OrderSettlePosition, nil
	case ActionIceberg:
		return OrderIceberg, nil
	case ActionStopLoss:
		return OrderStopLoss, nil
	case ActionStopLossLimit:
		return OrderStopLossLimit, nil
	case ActionTakeProfit:
		return OrderTakeProfit, nil
	case ActionTakeProfitLimit:
		return OrderTakeProfitLimit, nil
	case ActionTrailingStop:
		return OrderTrailingStop, nil
	case ActionTrailingStopLimit:
		return OrderTrailingStopLimit, nil
	default:
		return "", errnie.Err(
			errnie.Validation,
			"unsupported order action: "+string(actionType),
			nil,
		)
	}
}
