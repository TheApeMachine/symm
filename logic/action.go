package logic

import (
	"time"

	"github.com/theapemachine/errnie"
)

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
	Notional        float64      `yaml:"notional" json:"notional"`
	Offset          float64      `yaml:"offset" json:"offset"`
	Fraction        float64      `yaml:"fraction" json:"fraction"`
	EntryScore      float64      `yaml:"-" json:"entry_score"`
	EntryConfidence float64      `yaml:"-" json:"entry_confidence"`
	ReasonSource    SourceType   `yaml:"-" json:"reason_source"`
	ReasonCategory  CategoryType `yaml:"-" json:"reason_category"`
	BranchKey       string       `yaml:"-" json:"branch_key"`
	ActionID        string       `yaml:"-" json:"action_id"`
	DecisionID      string       `yaml:"-" json:"decision_id"`
	ClOrdID         string       `yaml:"-" json:"cl_ord_id"`
	Allowed         bool         `yaml:"-" json:"allowed"`
	Verdict         string       `yaml:"-" json:"verdict"`
	Reason          string       `yaml:"-" json:"reason"`
	DecisionAt      string       `yaml:"-" json:"decision_at"`
	RiskStamped     bool         `yaml:"-" json:"risk_stamped"`
	Story           StoryTrace   `yaml:"-" json:"story"`
}

func (action *Action) Allow(baseFraction float64) error {
	if action == nil {
		return errnie.Err(errnie.Validation, "logic: nil action", nil)
	}

	if action.Type == "" || action.Type == ActionNone {
		return errnie.Err(errnie.Validation, "logic: action type required", nil)
	}

	if action.Fraction <= 0 &&
		action.Notional <= 0 &&
		action.Quantity <= 0 &&
		!action.Type.IsExit() {
		action.Fraction = baseFraction
	}

	action.Allowed = true
	action.Verdict = "allow"
	action.DecisionAt = time.Now().UTC().Format(time.RFC3339Nano)
	action.RiskStamped = true
	return nil
}

func (action *Action) Block(reason string) {
	action.Allowed = false
	action.Verdict = "blocked"
	action.Reason = reason
	action.DecisionAt = time.Now().UTC().Format(time.RFC3339Nano)
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
