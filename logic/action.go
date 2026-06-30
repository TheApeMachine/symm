package logic

import (
	"math"

	"github.com/theapemachine/datura"
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
	strategy string,
) *Action {
	return &Action{
		Type:     actionType,
		Side:     side,
		Symbol:   symbol,
		Price:    price,
		Quantity: quantity,
		Offset:   offset,
	}
}

func actionForMatch(
	action *Action,
	symbol string,
	measurements []*datura.Artifact,
	group *ConditionGroup,
) (*Action, error) {
	if action == nil {
		return nil, errnie.Err(errnie.Validation, "logic: nil action", nil)
	}

	next := *action
	if next.Symbol == "" {
		next.Symbol = symbol
	}

	enrichActionEvidence(&next, symbol, measurements, group)

	if next.Side == SideBuy && !next.Type.IsExit() && next.EntryConfidence <= 0 {
		return nil, errnie.Err(
			errnie.Validation,
			"logic: buy action missing entry evidence for "+next.Symbol,
			nil,
		)
	}

	return &next, nil
}

func enrichActionEvidence(
	action *Action,
	targetSymbol string,
	measurements []*datura.Artifact,
	group *ConditionGroup,
) {
	if action == nil || group == nil || action.Type.IsExit() {
		return
	}

	confidence, source, category := actionConfidence(targetSymbol, measurements, group)

	if action.EntryConfidence <= 0 && confidence > 0 {
		action.EntryConfidence = confidence
	}

	if action.ReasonSource == SourceNone && source != SourceNone {
		action.ReasonSource = source
	}

	if action.ReasonCategory == CategoryTypeNone && category != CategoryTypeNone {
		action.ReasonCategory = category
	}
}

func actionConfidence(
	targetSymbol string,
	measurements []*datura.Artifact,
	group *ConditionGroup,
) (float64, SourceType, CategoryType) {
	if group == nil {
		return 0, SourceNone, CategoryTypeNone
	}

	return groupActionConfidence(targetSymbol, measurements, group)
}

func groupActionConfidence(
	targetSymbol string,
	measurements []*datura.Artifact,
	group *ConditionGroup,
) (float64, SourceType, CategoryType) {
	confidence := 0.0
	source := SourceNone
	category := CategoryTypeNone

	for _, condition := range group.Conditions {
		if condition.Type == ConditionIsFalse {
			continue
		}

		matched, matchErr := condition.Evaluate(targetSymbol, measurements, nil)
		if matchErr != nil || !matched {
			continue
		}

		next, ok := confidenceForOperand(targetSymbol, measurements, condition.Left)

		if !ok {
			continue
		}

		confidence, source, category = mergeActionEvidence(
			group.Boolean,
			confidence,
			source,
			category,
			next,
		)
	}

	for index := range group.Groups {
		nextConfidence, nextSource, nextCategory := groupActionConfidence(
			targetSymbol,
			measurements,
			&group.Groups[index],
		)

		if nextConfidence <= 0 || nextSource == SourceNone {
			continue
		}

		confidence, source, category = mergeActionEvidence(
			group.Boolean,
			confidence,
			source,
			category,
			actionEvidence{
				confidence: nextConfidence,
				source:     nextSource,
				category:   nextCategory,
			},
		)
	}

	return confidence, source, category
}

func mergeActionEvidence(
	boolType BooleanType,
	confidence float64,
	source SourceType,
	category CategoryType,
	next actionEvidence,
) (float64, SourceType, CategoryType) {
	switch boolType {
	case BooleanTypeOr:
		if next.confidence > confidence {
			confidence = next.confidence
			source = next.source
			category = next.category
		}
	default:
		if confidence == 0 || next.confidence < confidence {
			confidence = next.confidence
			source = next.source
			category = next.category
		}
	}

	return confidence, source, category
}

type actionEvidence struct {
	confidence float64
	source     SourceType
	category   CategoryType
}

func confidenceForOperand(
	targetSymbol string,
	measurements []*datura.Artifact,
	operand ConditionOperand,
) (actionEvidence, bool) {
	if operand.Source == SourceNone {
		return actionEvidence{}, false
	}

	switch operand.Type {
	case SubjectCategory, SubjectConfidence, SubjectStrength, SubjectSurprise:
	default:
		return actionEvidence{}, false
	}

	measurement, ok := measurementForSource(measurements, targetSymbol, operand.Source)

	if !ok {
		return actionEvidence{}, false
	}

	confidence := datura.Peek[float64](measurement, "output", "confidence")

	if confidence <= 0 || math.IsNaN(confidence) || math.IsInf(confidence, 0) {
		return actionEvidence{}, false
	}

	category := CategoryTypeNone
	if operand.Category != nil {
		category = operand.Category.Type
	}

	return actionEvidence{
		confidence: confidence,
		source:     operand.Source,
		category:   category,
	}, true
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
