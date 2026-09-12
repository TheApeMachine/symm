package learning

/*
Observation is the current reference and the past reference a target may use.
It is the shared wire payload of the target primitives.
*/
type Observation struct {
	Current float64
	Past    float64
}
