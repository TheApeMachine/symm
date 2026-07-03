package logic

import "github.com/theapemachine/errnie"

type ActionType string

const (
	ActionNone              ActionType = "none"
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
	Price           float64      `yaml:"price" json:"price"`
	Quantity        float64      `yaml:"quantity" json:"quantity"`
	Offset          float64      `yaml:"offset" json:"offset"`
	Fraction        float64      `yaml:"fraction" json:"fraction"`
	EntryConfidence float64      `yaml:"-" json:"entry_confidence"`
	ReasonSource    SourceType   `yaml:"-" json:"reason_source"`
	ReasonCategory  CategoryType `yaml:"-" json:"reason_category"`
	BranchKey       string       `yaml:"-" json:"branch_key"`
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

func (actionType ActionType) Protective() bool {
	switch actionType {
	case ActionStopLoss, ActionStopLossLimit,
		ActionTakeProfit, ActionTakeProfitLimit,
		ActionTrailingStop, ActionTrailingStopLimit:
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
	case ActionSettlePosition:
		return OrderSettlePosition, nil
	default:
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"logic: unsupported action type: "+string(actionType),
			nil,
		))
	}
}
