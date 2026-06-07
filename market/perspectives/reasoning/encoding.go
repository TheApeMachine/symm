package reasoning

import (
	"fmt"

	"go.yaml.in/yaml/v3"

	"github.com/theapemachine/symm/kraken/trading"
	"github.com/theapemachine/symm/market/perspectives/types"
)

var subjectNames = map[Subject]string{
	SubjectNone:     "none",
	SubjectSignal:   "signal",
	SubjectRegime:   "regime",
	SubjectPosition: "position",
	SubjectPrice:    "price",
	SubjectVolume:   "volume",
	SubjectSpread:   "spread",
	SubjectElapsed:  "elapsed",
}

var comparisonNames = map[Comparison]string{
	ComparisonNone:        "none",
	ComparisonAtLeast:     "at_least",
	ComparisonAtMost:      "at_most",
	ComparisonAbove:       "above",
	ComparisonBelow:       "below",
	ComparisonEquals:      "equals",
	ComparisonRoseBy:      "rose_by",
	ComparisonFellBy:      "fell_by",
	ComparisonCrossedUp:   "crossed_up",
	ComparisonCrossedDown: "crossed_down",
}

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
	UnitStrength:         "strength",
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

func (subject Subject) MarshalYAML() (any, error) {
	return types.MarshalEnum(subject, subjectNames)
}

func (subject *Subject) UnmarshalYAML(node *yaml.Node) error {
	return types.UnmarshalEnum(node, subject, subjectNames)
}

func (comparison Comparison) MarshalYAML() (any, error) {
	return types.MarshalEnum(comparison, comparisonNames)
}

func (comparison *Comparison) UnmarshalYAML(node *yaml.Node) error {
	return types.UnmarshalEnum(node, comparison, comparisonNames)
}

func (unit UnitType) String() string {
	name, ok := unitNames[unit]

	if ok {
		return name
	}

	return fmt.Sprintf("unit_%d", unit)
}

func (unit UnitType) MarshalYAML() (any, error) {
	return types.MarshalEnum(unit, unitNames)
}

func (unit UnitType) MarshalJSON() ([]byte, error) {
	return types.MarshalEnumJSON(unit, unitNames)
}

func (unit *UnitType) UnmarshalYAML(value *yaml.Node) error {
	return types.UnmarshalEnum(value, unit, unitNames)
}

func (unit *UnitType) UnmarshalJSON(data []byte) error {
	return types.UnmarshalEnumJSON(data, unit, unitNames)
}

func (condition ConditionType) MarshalYAML() (any, error) {
	return types.MarshalEnum(condition, conditionNames)
}

func (condition ConditionType) MarshalJSON() ([]byte, error) {
	return types.MarshalEnumJSON(condition, conditionNames)
}

func (condition *ConditionType) UnmarshalYAML(value *yaml.Node) error {
	return types.UnmarshalEnum(value, condition, conditionNames)
}

func (condition *ConditionType) UnmarshalJSON(data []byte) error {
	return types.UnmarshalEnumJSON(data, condition, conditionNames)
}

func (action ActionType) String() string {
	name, ok := actionNames[action]

	if ok {
		return name
	}

	return fmt.Sprintf("action_%d", action)
}

func (action ActionType) MarshalYAML() (any, error) {
	return types.MarshalEnum(action, actionNames)
}

func (action ActionType) MarshalJSON() ([]byte, error) {
	return types.MarshalEnumJSON(action, actionNames)
}

func (action *ActionType) UnmarshalYAML(value *yaml.Node) error {
	return types.UnmarshalEnum(value, action, actionNames)
}

func (action *ActionType) UnmarshalJSON(data []byte) error {
	return types.UnmarshalEnumJSON(data, action, actionNames)
}

/*
UnmarshalYAML lets `do:` be a bare action ("do: iceberg") or the parameterized
object ("do: { type: stop_loss, offset: 0.015 }").
*/
func (act *Act) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		return node.Decode(&act.Type)
	}

	var raw struct {
		Type     ActionType   `yaml:"type"`
		Side     trading.Side `yaml:"side,omitempty"`
		Offset   float64      `yaml:"offset"`
		Fraction float64      `yaml:"fraction"`
	}

	if err := node.Decode(&raw); err != nil {
		return err
	}

	act.Type = raw.Type
	act.Side = raw.Side
	act.Offset = raw.Offset
	act.Fraction = raw.Fraction

	return nil
}

/*
MarshalYAML is the inverse of the reader above: a bare action ("do: iceberg") when
there is no per-node offset or side, and the object form when there is.
*/
func (act Act) MarshalYAML() (any, error) {
	if act.Offset == 0 && act.Side == "" && act.Fraction == 0 {
		return act.Type, nil
	}

	return struct {
		Type     ActionType   `yaml:"type"`
		Side     trading.Side `yaml:"side,omitempty"`
		Offset   float64      `yaml:"offset,omitempty"`
		Fraction float64      `yaml:"fraction,omitempty"`
	}{
		Type:     act.Type,
		Side:     act.Side,
		Offset:   act.Offset,
		Fraction: act.Fraction,
	}, nil
}

type reasoningDocument struct {
	Version  int       `yaml:"version"`
	Branches []Thought `yaml:"branches"`
}

/*
ParseThoughts decodes a version-2 playbook document into the reasoning forest.
*/
func ParseThoughts(raw []byte) ([]Thought, error) {
	var document reasoningDocument

	if err := yaml.Unmarshal(raw, &document); err != nil {
		return nil, err
	}

	StampStrategies(document.Branches)

	return document.Branches, nil
}

/*
MarshalThoughts encodes a reasoning forest back into a playbook document — the
inverse of ParseThoughts. The optimizer writes the trees it discovers this way, and
a hand-written playbook round-trips through it unchanged.
*/
func MarshalThoughts(thoughts []Thought, version int) ([]byte, error) {
	return yaml.Marshal(reasoningDocument{Version: version, Branches: thoughts})
}
