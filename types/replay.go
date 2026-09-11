package types

import "github.com/theapemachine/symm/nomagique/data"

/*
ReplayFragment carries one historical excursion's frames together with factual
boundary metadata (anchor index B and extremum index) and instrument symbol.
This boundary metadata is strictly owned by the replay controller and evaluation;
it is NEVER leaked into the learner's observation context.
*/
type ReplayFragment struct {
	Frames        [][]*data.Measurement[float64]
	Surfaces      []*ExecutionSurface
	Symbol        string
	AnchorIndex   int
	ExtremumIndex int
}
