package advisor

type PredictedType uint8
type PredictedMove uint8
type UnitType uint8

const (
	METRIC PredictedType = iota
	TICKERS
	TRADES
	LEVEL3

	NOMOVE PredictedMove = iota
	INCREASE
	DECREASE
	STAGNATE
	DISSOLVE

	PERCENT UnitType = iota
)

type Falsifiable struct {
	Label string
	Type  PredictedType
	Move  PredictedMove
	Value float64
	Unit  UnitType
}

type Prediction struct {
	Support    *Falsifiable
	Contradict *Falsifiable
}

type Class struct {
	Label       string
	Predictions []*Prediction
}

type Feature struct {
	Clock string
	Keys  []string
	Class *Class
}

func NewFeature(
	clock string,
	keys []string,
	class *Class,
) *Feature {
	return &Feature{
		Clock: clock,
		Keys:  keys,
		Class: class,
	}
}
