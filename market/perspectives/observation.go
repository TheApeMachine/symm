package perspectives

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

type Observation struct {
	ObservationType ObservationType
	Value           float64
	Branch          *map[ObservationType]*Tree
}
