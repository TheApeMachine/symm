package logic

import (
	"github.com/theapemachine/errnie"
)

type SubjectType string

const (
	SubjectNone       SubjectType = ""
	SubjectCategory   SubjectType = "category"
	SubjectRegime     SubjectType = "regime"
	SubjectPosition   SubjectType = "position"
	SubjectHolding    SubjectType = "holding"
	SubjectPrice      SubjectType = "price"
	SubjectVolume     SubjectType = "volume"
	SubjectSpread     SubjectType = "spread"
	SubjectElapsed    SubjectType = "elapsed"
	SubjectStrength   SubjectType = "strength"
	SubjectConfidence SubjectType = "confidence"
	SubjectSurprise   SubjectType = "surprise"
)

type Subject struct {
	Source     SourceType      `yaml:"source" json:"source"`
	Type       SubjectType     `yaml:"type" json:"type"`
	Category   *Category       `yaml:"category" json:"category"`
	Regime     *Regime         `yaml:"regime" json:"regime"`
	Position   *Position       `yaml:"position" json:"position"`
	Holding    *HoldingSubject `yaml:"holding" json:"holding"`
	Price      float64         `yaml:"price" json:"price"`
	Volume     float64         `yaml:"volume" json:"volume"`
	Spread     float64         `yaml:"spread" json:"spread"`
	Elapsed    float64         `yaml:"elapsed" json:"elapsed"`
	Strength   float64         `yaml:"strength" json:"strength"`
	Confidence float64         `yaml:"confidence" json:"confidence"`
	Surprise   float64         `yaml:"surprise" json:"surprise"`

	confidenceUsesBaseline      bool
	confidenceUsesEntryBaseline bool
	confidenceUsesExitBaseline  bool
	surpriseUsesBaseline        bool
}

func NewSubject(
	source SourceType,
	subjectType SubjectType,
	category *Category,
	regime *Regime,
	position *Position,
	price float64,
	volume float64,
	spread float64,
	elapsed float64,
	strength float64,
	confidence float64,
	surprise float64,
) *Subject {
	return &Subject{
		Source:     source,
		Type:       subjectType,
		Category:   category,
		Regime:     regime,
		Position:   position,
		Price:      price,
		Volume:     volume,
		Spread:     spread,
		Elapsed:    elapsed,
		Strength:   strength,
		Confidence: confidence,
		Surprise:   surprise,
	}
}

func (subject *Subject) isEnumerated() bool {
	switch subject.Type {
	case SubjectCategory, SubjectRegime, SubjectPosition, SubjectHolding:
		return true
	default:
		return false
	}
}

/*
anchorsTimeline reports whether a subject match should burn measurements for child branches.
Inventory and entry attribution are state gates, not temporal anchors.
*/
func (subject *Subject) anchorsTimeline() bool {
	switch subject.Type {
	case SubjectHolding:
		return false
	default:
		return true
	}
}

func (subject *Subject) Evaluate(
	measurement Measurement, holdings *Holdings,
) (bool, error) {
	switch subject.Type {
	case SubjectCategory:
		if subject.Category == nil {
			return false, nil
		}

		return subject.Category.Type == measurement.Category, nil
	case SubjectRegime:
		if subject.Regime == nil {
			return false, nil
		}

		return subject.Regime.Type == measurement.Regime, nil
	case SubjectPosition:
		if subject.Position == nil {
			return false, nil
		}

		return subject.Position.Type == measurement.Position, nil
	case SubjectHolding:
		if subject.Holding == nil {
			return false, nil
		}

		if holdings == nil {
			return false, errnie.Err(
				errnie.Validation,
				"logic: holdings required for holding subject",
				nil,
			)
		}

		held := holdings.IsHolding(measurement.Symbol)

		return held == subject.Holding.Held, nil
	case SubjectPrice:
		return subject.Price == measurement.Price, nil
	case SubjectVolume:
		return subject.Volume == measurement.Volume, nil
	case SubjectSpread:
		return subject.Spread == measurement.Spread, nil
	case SubjectElapsed:
		return subject.Elapsed == measurement.Elapsed, nil
	case SubjectStrength:
		return subject.Strength == measurement.Strength, nil
	case SubjectConfidence:
		return subject.Confidence == measurement.Confidence, nil
	case SubjectSurprise:
		return subject.Surprise == measurement.Surprise, nil
	}

	return false, nil
}

func (subject *Subject) valueFrom(measurement Measurement) (float64, bool) {
	switch subject.Type {
	case SubjectPrice:
		return measurement.Price, true
	case SubjectVolume:
		return measurement.Volume, true
	case SubjectSpread:
		return measurement.Spread, true
	case SubjectElapsed:
		return measurement.Elapsed, true
	case SubjectStrength:
		return measurement.Strength, true
	case SubjectConfidence:
		return measurement.Confidence, true
	case SubjectSurprise:
		return measurement.Surprise, true
	default:
		return 0, false
	}
}

func (subject *Subject) threshold() (float64, bool) {
	switch subject.Type {
	case SubjectPrice:
		return subject.Price, true
	case SubjectVolume:
		return subject.Volume, true
	case SubjectSpread:
		return subject.Spread, true
	case SubjectElapsed:
		return subject.Elapsed, true
	case SubjectStrength:
		return subject.Strength, true
	case SubjectConfidence:
		return subject.Confidence, true
	case SubjectSurprise:
		return subject.Surprise, true
	default:
		return 0, false
	}
}
