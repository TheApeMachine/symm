package perspectives

import (
	"fmt"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"
)

var unitNames = map[UnitType]string{
	UnitNone:             "none",
	UnitPercentage:       "percentage",
	UnitPips:             "pips",
	UnitPoints:           "points",
	UnitTicks:            "ticks",
	UnitTimeYears:        "time_years",
	UnitTimeMonths:       "time_months",
	UnitTimeWeeks:        "time_weeks",
	UnitTimeDays:         "time_days",
	UnitTimeHours:        "time_hours",
	UnitTimeMinutes:      "time_minutes",
	UnitTimeSeconds:      "time_seconds",
	UnitTimeMilliseconds: "time_milliseconds",
	UnitTimeMicroseconds: "time_microseconds",
	UnitTimeNanoseconds:  "time_nanoseconds",
	UnitConfidence:       "confidence",
	UnitSNR:              "snr",
}

var conditionNames = map[ConditionType]string{
	ConditionNone:                 "none",
	ConditionIsTrue:               "true",
	ConditionIsFalse:              "false",
	ConditionIsEqual:              "==",
	ConditionIsNotEqual:           "!=",
	ConditionIsGreaterThan:        ">",
	ConditionIsLessThan:           "<",
	ConditionIsGreaterThanOrEqual: ">=",
	ConditionIsLessThanOrEqual:    "<=",
}

var actionNames = map[ActionType]string{
	ActionNone:              "none",
	ActionLimit:             "limit",
	ActionMarket:            "market",
	ActionIceberg:           "iceberg",
	ActionStopLoss:          "stop_loss",
	ActionStopLossLimit:     "stop_loss_limit",
	ActionTakeProfit:        "take_profit",
	ActionTakeProfitLimit:   "take_profit_limit",
	ActionTrailingStop:      "trailing_stop",
	ActionTrailingStopLimit: "trailing_stop_limit",
	ActionSettlePosition:    "settle_position",
}

var observationNames = map[ObservationType]string{
	ObservationNone:          "none",
	ObservationHasStarted:    "has_started",
	ObservationHasContinued:  "has_continued",
	ObservationHasEnded:      "has_ended",
	ObservationHasDoneBefore: "has_done_before",
	ObservationHolding:       "holding",
	ObservationNotHolding:    "not_holding",
}

var regimeNames = map[Regime]string{
	RegimeNone:     "none",
	RegimeDead:     "dead",
	RegimeChoppy:   "choppy",
	RegimeTrending: "trending",
	RegimeBullish:  "bullish",
	RegimeBearish:  "bearish",
}

func (unit UnitType) MarshalYAML() (any, error) {
	return marshalEnum(unit, unitNames)
}

func (unit UnitType) MarshalJSON() ([]byte, error) {
	return marshalEnumJSON(unit, unitNames)
}

func (unit *UnitType) UnmarshalYAML(value *yaml.Node) error {
	return unmarshalEnum(value, unit, unitNames)
}

func (condition ConditionType) MarshalYAML() (any, error) {
	return marshalEnum(condition, conditionNames)
}

func (condition ConditionType) MarshalJSON() ([]byte, error) {
	return marshalEnumJSON(condition, conditionNames)
}

func (condition *ConditionType) UnmarshalYAML(value *yaml.Node) error {
	return unmarshalEnum(value, condition, conditionNames)
}

func (action ActionType) MarshalYAML() (any, error) {
	return marshalEnum(action, actionNames)
}

func (action ActionType) MarshalJSON() ([]byte, error) {
	return marshalEnumJSON(action, actionNames)
}

func (action *ActionType) UnmarshalYAML(value *yaml.Node) error {
	return unmarshalEnum(value, action, actionNames)
}

func (observation ObservationType) MarshalYAML() (any, error) {
	return marshalEnum(observation, observationNames)
}

func (observation ObservationType) MarshalJSON() ([]byte, error) {
	return marshalEnumJSON(observation, observationNames)
}

func (observation *ObservationType) UnmarshalYAML(value *yaml.Node) error {
	return unmarshalEnum(value, observation, observationNames)
}

func (regime Regime) MarshalYAML() (any, error) {
	return marshalEnum(regime, regimeNames)
}

func (regime Regime) MarshalJSON() ([]byte, error) {
	return marshalEnumJSON(regime, regimeNames)
}

func (regime *Regime) UnmarshalYAML(value *yaml.Node) error {
	return unmarshalEnum(value, regime, regimeNames)
}

func marshalEnum[enumType comparable](
	value enumType, names map[enumType]string,
) (any, error) {
	name, ok := names[value]

	if ok {
		return name, nil
	}

	return nil, fmt.Errorf("perspectives: unknown enum value %v", value)
}

func marshalEnumJSON[enumType comparable](
	value enumType, names map[enumType]string,
) ([]byte, error) {
	name, ok := names[value]

	if ok {
		return []byte(strconv.Quote(name)), nil
	}

	return nil, fmt.Errorf("perspectives: unknown enum value %v", value)
}

func unmarshalEnum[enumType ~uint8](
	node *yaml.Node, target *enumType, names map[enumType]string,
) error {
	if node.Tag == "!!int" {
		return unmarshalNumericEnum(node, target)
	}

	value := normalizeEnumName(node.Value)

	for enumValue, name := range names {
		if normalizeEnumName(name) == value {
			*target = enumValue

			return nil
		}
	}

	return fmt.Errorf("perspectives: unknown enum value %q", node.Value)
}

func unmarshalNumericEnum[enumType ~uint8](
	node *yaml.Node, target *enumType,
) error {
	parsed, err := strconv.ParseUint(node.Value, 10, 8)

	if err != nil {
		return err
	}

	*target = enumType(parsed)

	return nil
}

func normalizeEnumName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "-", "_")

	return value
}
