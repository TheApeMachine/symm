package logic

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

func unmarshalEnum(
	node *yaml.Node,
	table map[string]uint8,
	destination *uint8,
) error {
	var raw string

	if err := node.Decode(&raw); err != nil {
		var number uint8

		if err := node.Decode(&number); err != nil {
			return err
		}

		*destination = number

		return nil
	}

	value, ok := table[raw]

	if !ok {
		return fmt.Errorf("logic: unknown yaml enum value %q", raw)
	}

	*destination = value

	return nil
}

func (conditionType *ConditionType) UnmarshalYAML(node *yaml.Node) error {
	return unmarshalEnum(node, map[string]uint8{
		"is_true":                  uint8(ConditionIsTrue),
		"is_false":                 uint8(ConditionIsFalse),
		"is_equal":                 uint8(ConditionIsEqual),
		"is_not_equal":             uint8(ConditionIsNotEqual),
		"is_greater_than":          uint8(ConditionIsGreaterThan),
		"is_less_than":             uint8(ConditionIsLessThan),
		"is_greater_than_or_equal": uint8(ConditionIsGreaterThanOrEqual),
		"is_less_than_or_equal":    uint8(ConditionIsLessThanOrEqual),
		"is_within":                uint8(ConditionIsWithin),
		"is_not_within":            uint8(ConditionIsNotWithin),
	}, (*uint8)(conditionType))
}

func (subjectType *SubjectType) UnmarshalYAML(node *yaml.Node) error {
	return unmarshalEnum(node, map[string]uint8{
		"category":   uint8(SubjectCategory),
		"regime":     uint8(SubjectRegime),
		"position":   uint8(SubjectPosition),
		"holding":    uint8(SubjectHolding),
		"price":      uint8(SubjectPrice),
		"volume":     uint8(SubjectVolume),
		"spread":     uint8(SubjectSpread),
		"elapsed":    uint8(SubjectElapsed),
		"strength":   uint8(SubjectStrength),
		"confidence": uint8(SubjectConfidence),
		"surprise":   uint8(SubjectSurprise),
	}, (*uint8)(subjectType))
}

func (booleanType *BooleanType) UnmarshalYAML(node *yaml.Node) error {
	return unmarshalEnum(node, map[string]uint8{
		"and": uint8(BooleanTypeAnd),
		"or":  uint8(BooleanTypeOr),
	}, (*uint8)(booleanType))
}

func (actionType *ActionType) UnmarshalYAML(node *yaml.Node) error {
	return unmarshalEnum(node, map[string]uint8{
		"limit":               uint8(ActionLimit),
		"market":              uint8(ActionMarket),
		"iceberg":             uint8(ActionIceberg),
		"stop_loss":           uint8(ActionStopLoss),
		"stop_loss_limit":     uint8(ActionStopLossLimit),
		"take_profit":         uint8(ActionTakeProfit),
		"take_profit_limit":   uint8(ActionTakeProfitLimit),
		"trailing_stop":       uint8(ActionTrailingStop),
		"trailing_stop_limit": uint8(ActionTrailingStopLimit),
		"settle_position":     uint8(ActionSettlePosition),
	}, (*uint8)(actionType))
}
