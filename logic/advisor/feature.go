package advisor

type PredictedType uint8
type PredictedMove uint8
type UnitType uint8

const (
	METRIC PredictedType = iota
	TICKERS
	TRADES
	LEVEL3
)

const (
	NOMOVE PredictedMove = iota
	INCREASE
	DECREASE
	STAGNATE
	EXPAND
	DISSOLVE
)

const (
	PERCENT UnitType = iota
	RAW
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

/*
NewMetricPrediction declares opposing direction-of-change observations for one
adaptive metric. Zero Value means any strict movement in that direction.
*/
func NewMetricPrediction(
	label string,
	support PredictedMove,
	contradict PredictedMove,
) *Prediction {
	return &Prediction{
		Support: &Falsifiable{
			Label: label,
			Type:  METRIC,
			Move:  support,
			Unit:  RAW,
		},
		Contradict: &Falsifiable{
			Label: label,
			Type:  METRIC,
			Move:  contradict,
			Unit:  RAW,
		},
	}
}

type Class struct {
	Label       string
	Within      uint64
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
