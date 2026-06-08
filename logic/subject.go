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
)

type Subject struct {
	Type     SubjectType
	Category *Category
	Regime   *Regime
	Position *Position
	Price    float64
	Volume   float64
	Spread   float64
	Elapsed  float64
}

func NewSubject(
	subjectType SubjectType,
	category *Category,
	regime *Regime,
	position *Position,
	price float64,
	volume float64,
	spread float64,
	elapsed float64,
) *Subject {
	return &Subject{
		Type:     subjectType,
		Category: category,
		Regime:   regime,
		Position: position,
		Price:    price,
		Volume:   volume,
		Spread:   spread,
		Elapsed:  elapsed,
	}
}

func (subject *Subject) Evaluate(measurement Measurement) bool {
	switch subject.Type {
	case SubjectCategory:
		return subject.Category.Type == measurement.Category
	case SubjectRegime:
		return subject.Regime.Type == measurement.Regime
	case SubjectPosition:
		return subject.Position.Type == measurement.Position
	case SubjectPrice:
		return subject.Price == measurement.Price
	case SubjectVolume:
		return subject.Volume == measurement.Volume
	case SubjectSpread:
		return subject.Spread == measurement.Spread
	case SubjectElapsed:
		return subject.Elapsed == measurement.Elapsed
	}

	return false
}
