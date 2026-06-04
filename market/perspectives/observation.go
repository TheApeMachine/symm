package perspectives

type ObservedType uint8

const (
	ObservedNone ObservedType = iota
	ObservedPrice
	ObservedVolume
	ObservedTime
)

type Observed struct {
	ObservedType         ObservedType
	Values               []float64
	ConditionType        ConditionType
	ConditionBooleanType ConditionBooleanType
}

type ObservationType uint8

const (
	ObservationNone ObservationType = iota
	ObservationHasStarted
	ObservationHasContinued
	ObservationHasEnded
	ObservationHasDoneBefore
	ObservationHolding
	ObservationNotHolding
)

