package logic

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

const thresholdBaselineSentinel = "baseline"

const thresholdEntryBaselineSentinel = "entry_baseline"

const thresholdExitBaselineSentinel = "exit_baseline"

type thresholdSentinelKind int

const (
	thresholdSentinelNone thresholdSentinelKind = iota
	thresholdSentinelStaticBaseline
	thresholdSentinelEntryBaseline
	thresholdSentinelExitBaseline
)

func decodeThresholdValue(raw any) (float64, thresholdSentinelKind, error) {
	if raw == nil {
		return 0, thresholdSentinelNone, nil
	}

	switch typed := raw.(type) {
	case string:
		switch typed {
		case thresholdBaselineSentinel:
			return 0, thresholdSentinelStaticBaseline, nil
		case thresholdEntryBaselineSentinel:
			return 0, thresholdSentinelEntryBaseline, nil
		case thresholdExitBaselineSentinel:
			return 0, thresholdSentinelExitBaseline, nil
		default:
			return 0, thresholdSentinelNone, fmt.Errorf(
				"logic: unsupported threshold sentinel %q",
				typed,
			)
		}
	case float64:
		return typed, thresholdSentinelNone, nil
	case int:
		return float64(typed), thresholdSentinelNone, nil
	case int64:
		return float64(typed), thresholdSentinelNone, nil
	default:
		return 0, thresholdSentinelNone, fmt.Errorf(
			"logic: unsupported threshold type %T",
			raw,
		)
	}
}

func (subject *Subject) UnmarshalYAML(value *yaml.Node) error {
	type subjectFields struct {
		Source     SourceType      `yaml:"source"`
		Type       SubjectType     `yaml:"type"`
		Category   *Category       `yaml:"category"`
		Regime     *Regime         `yaml:"regime"`
		Position   *Position       `yaml:"position"`
		Holding    *HoldingSubject `yaml:"holding"`
		Price      float64         `yaml:"price"`
		Volume     float64         `yaml:"volume"`
		Spread     float64         `yaml:"spread"`
		Elapsed    float64         `yaml:"elapsed"`
		Strength   float64         `yaml:"strength"`
		Confidence any             `yaml:"confidence"`
		Surprise   any             `yaml:"surprise"`
	}

	var fields subjectFields

	if err := value.Decode(&fields); err != nil {
		return err
	}

	subject.Source = fields.Source
	subject.Type = fields.Type
	subject.Category = fields.Category
	subject.Regime = fields.Regime
	subject.Position = fields.Position
	subject.Holding = fields.Holding
	subject.Price = fields.Price
	subject.Volume = fields.Volume
	subject.Spread = fields.Spread
	subject.Elapsed = fields.Elapsed
	subject.Strength = fields.Strength

	confidence, confidenceSentinel, err := decodeThresholdValue(fields.Confidence)

	if err != nil {
		return err
	}

	subject.Confidence = confidence

	switch confidenceSentinel {
	case thresholdSentinelStaticBaseline:
		subject.confidenceUsesBaseline = true
	case thresholdSentinelEntryBaseline:
		subject.confidenceUsesEntryBaseline = true
	case thresholdSentinelExitBaseline:
		subject.confidenceUsesExitBaseline = true
	}

	surprise, surpriseSentinel, err := decodeThresholdValue(fields.Surprise)

	if err != nil {
		return err
	}

	subject.Surprise = surprise

	if surpriseSentinel == thresholdSentinelStaticBaseline {
		subject.surpriseUsesBaseline = true
	}

	return nil
}
