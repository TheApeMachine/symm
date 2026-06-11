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

func yamlNodePresent(node yaml.Node) bool {
	return node.Kind != 0 && !node.IsZero()
}

func inferSubjectType(
	category *Category,
	regime *Regime,
	position *Position,
	holding *HoldingSubject,
	priceNode yaml.Node,
	volumeNode yaml.Node,
	spreadNode yaml.Node,
	elapsedNode yaml.Node,
	strengthNode yaml.Node,
	confidenceNode yaml.Node,
	surpriseNode yaml.Node,
) SubjectType {
	if category != nil {
		return SubjectCategory
	}

	if regime != nil {
		return SubjectRegime
	}

	if position != nil {
		return SubjectPosition
	}

	if holding != nil {
		return SubjectHolding
	}

	if yamlNodePresent(confidenceNode) {
		return SubjectConfidence
	}

	if yamlNodePresent(surpriseNode) {
		return SubjectSurprise
	}

	if yamlNodePresent(priceNode) {
		return SubjectPrice
	}

	if yamlNodePresent(volumeNode) {
		return SubjectVolume
	}

	if yamlNodePresent(spreadNode) {
		return SubjectSpread
	}

	if yamlNodePresent(elapsedNode) {
		return SubjectElapsed
	}

	if yamlNodePresent(strengthNode) {
		return SubjectStrength
	}

	return SubjectNone
}

func (subject *Subject) UnmarshalYAML(node *yaml.Node) error {
	type subjectFields struct {
		Source     SourceType      `yaml:"source"`
		Type       SubjectType     `yaml:"type"`
		Category   *Category       `yaml:"category"`
		Regime     *Regime         `yaml:"regime"`
		Position   *Position       `yaml:"position"`
		Holding    *HoldingSubject `yaml:"holding"`
		Price      yaml.Node       `yaml:"price"`
		Volume     yaml.Node       `yaml:"volume"`
		Spread     yaml.Node       `yaml:"spread"`
		Elapsed    yaml.Node       `yaml:"elapsed"`
		Strength   yaml.Node       `yaml:"strength"`
		Confidence yaml.Node       `yaml:"confidence"`
		Surprise   yaml.Node       `yaml:"surprise"`
	}

	fields := subjectFields{}

	if err := node.Decode(&fields); err != nil {
		return err
	}

	subject.Source = fields.Source
	subject.Type = fields.Type
	subject.Category = fields.Category
	subject.Regime = fields.Regime
	subject.Position = fields.Position
	subject.Holding = fields.Holding

	if err := fields.Price.Decode(&subject.Price); err != nil {
		return fmt.Errorf("logic: subject price: %w", err)
	}

	if err := fields.Volume.Decode(&subject.Volume); err != nil {
		return fmt.Errorf("logic: subject volume: %w", err)
	}

	if err := fields.Spread.Decode(&subject.Spread); err != nil {
		return fmt.Errorf("logic: subject spread: %w", err)
	}

	if err := fields.Elapsed.Decode(&subject.Elapsed); err != nil {
		return fmt.Errorf("logic: subject elapsed: %w", err)
	}

	if err := fields.Strength.Decode(&subject.Strength); err != nil {
		return fmt.Errorf("logic: subject strength: %w", err)
	}

	if err := fields.Confidence.Decode(&subject.Confidence); err != nil {
		return fmt.Errorf("logic: subject confidence: %w", err)
	}

	if err := fields.Surprise.Decode(&subject.Surprise); err != nil {
		return fmt.Errorf("logic: subject surprise: %w", err)
	}

	if subject.Type == SubjectNone {
		subject.Type = inferSubjectType(
			fields.Category,
			fields.Regime,
			fields.Position,
			fields.Holding,
			fields.Price,
			fields.Volume,
			fields.Spread,
			fields.Elapsed,
			fields.Strength,
			fields.Confidence,
			fields.Surprise,
		)
	}

	return nil
}
