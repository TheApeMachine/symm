package types

import "github.com/theapemachine/symm/nomagique/data"

/*
ReplayFragment carries one historical excursion's frames together with factual
boundary metadata (anchor index B and extremum index C) and instrument symbol.
The excursion leg [B, C] has already been verified by Hindsight to have crossed
friction, eliminating any need for embedded prices or simulated economies.
*/
type ReplayFragment struct {
	Frames        [][]*data.Measurement[float64]
	Symbol        string
	AnchorIndex   int
	ExtremumIndex int
}
