package manifold

/*
PilotWaveProjection is the price-time slice of the coherence field and its
guidance velocity used by the terminal fluid canvas.
*/
type PilotWaveProjection struct {
	Mag2 [][]float64
	VelX [][]float64
	VelZ [][]float64
}
