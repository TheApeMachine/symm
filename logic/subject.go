package logic

type SubjectType uint8

const (
	SubjectNone SubjectType = iota
	SubjectCategory
	SubjectRegime
	SubjectPosition
	SubjectPrice
	SubjectVolume
	SubjectSpread
	SubjectElapsed
	SubjectStrength
	SubjectConfidence
	SubjectSurprise
)

type Subject struct {
	Source     SourceType  `yaml:"source"`
	Type       SubjectType `yaml:"type"`
	Category   *Category   `yaml:"category"`
	Regime     *Regime     `yaml:"regime"`
	Position   *Position   `yaml:"position"`
	Price      float64     `yaml:"price"`
	Volume     float64     `yaml:"volume"`
	Spread     float64     `yaml:"spread"`
	Elapsed    float64     `yaml:"elapsed"`
	Strength   float64     `yaml:"strength"`
	Confidence float64     `yaml:"confidence"`
	Surprise   float64     `yaml:"surprise"`
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
	case SubjectCategory, SubjectRegime, SubjectPosition:
		return true
	default:
		return false
	}
}

func (subject *Subject) Evaluate(measurement Measurement) bool {
	switch subject.Type {
	case SubjectCategory:
		if subject.Category == nil {
			return false
		}

		if subject.Category.Type != measurement.Category {
			return false
		}

		if subject.Category.Confidence > 0 &&
			measurement.Confidence < subject.Category.Confidence {
			return false
		}

		if subject.Category.Surprise > 0 &&
			measurement.Surprise < subject.Category.Surprise {
			return false
		}

		return true
	case SubjectRegime:
		if subject.Regime == nil {
			return false
		}

		return subject.Regime.Type == measurement.Regime
	case SubjectPosition:
		if subject.Position == nil {
			return false
		}

		return subject.Position.Type == measurement.Position
	case SubjectPrice:
		return subject.Price == measurement.Price
	case SubjectVolume:
		return subject.Volume == measurement.Volume
	case SubjectSpread:
		return subject.Spread == measurement.Spread
	case SubjectElapsed:
		return subject.Elapsed == measurement.Elapsed
	case SubjectStrength:
		return subject.Strength == measurement.Strength
	case SubjectConfidence:
		return subject.Confidence == measurement.Confidence
	case SubjectSurprise:
		return subject.Surprise == measurement.Surprise
	}

	return false
}

func (subject *Subject) enumMatches(measurement Measurement) bool {
	return subject.Evaluate(measurement)
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
