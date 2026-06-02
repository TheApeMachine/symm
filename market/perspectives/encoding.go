package perspectives

import (
	"encoding/json"
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

func (unit *UnitType) UnmarshalJSON(data []byte) error {
	return unmarshalEnumJSON(data, unit, unitNames)
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

func (condition *ConditionType) UnmarshalJSON(data []byte) error {
	return unmarshalEnumJSON(data, condition, conditionNames)
}

func (action ActionType) String() string {
	name, ok := actionNames[action]

	if ok {
		return name
	}

	return fmt.Sprintf("action_%d", action)
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

func (action *ActionType) UnmarshalJSON(data []byte) error {
	return unmarshalEnumJSON(data, action, actionNames)
}

func (observation ObservationType) String() string {
	name, ok := observationNames[observation]

	if ok {
		return name
	}

	return fmt.Sprintf("observation_%d", observation)
}

func (unit UnitType) String() string {
	name, ok := unitNames[unit]

	if ok {
		return name
	}

	return fmt.Sprintf("unit_%d", unit)
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

func (observation *ObservationType) UnmarshalJSON(data []byte) error {
	return unmarshalEnumJSON(data, observation, observationNames)
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

func (regime *Regime) UnmarshalJSON(data []byte) error {
	return unmarshalEnumJSON(data, regime, regimeNames)
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
		return unmarshalNumericEnum(node, target, names)
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

func unmarshalEnumJSON[enumType ~uint8](
	data []byte, target *enumType, names map[enumType]string,
) error {
	trimmed := strings.TrimSpace(string(data))

	if trimmed == "" {
		return fmt.Errorf("perspectives: empty enum value")
	}

	if trimmed[0] != '"' {
		parsed, err := strconv.ParseUint(trimmed, 10, 8)

		if err != nil {
			return err
		}

		return assignNumericEnum(enumType(parsed), target, names)
	}

	var name string

	if err := json.Unmarshal(data, &name); err != nil {
		return err
	}

	value := normalizeEnumName(name)

	for enumValue, enumName := range names {
		if normalizeEnumName(enumName) == value {
			*target = enumValue

			return nil
		}
	}

	return fmt.Errorf("perspectives: unknown enum value %q", name)
}

func unmarshalNumericEnum[enumType ~uint8](
	node *yaml.Node, target *enumType, names map[enumType]string,
) error {
	parsed, err := strconv.ParseUint(node.Value, 10, 8)

	if err != nil {
		return err
	}

	return assignNumericEnum(enumType(parsed), target, names)
}

func assignNumericEnum[enumType ~uint8](
	value enumType, target *enumType, names map[enumType]string,
) error {
	if _, ok := names[value]; !ok {
		return fmt.Errorf("perspectives: unknown enum value %d", value)
	}

	*target = value

	return nil
}

func normalizeEnumName(value string) string {
	value = strings.TrimSpace(value)
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "-", "_")

	return value
}
