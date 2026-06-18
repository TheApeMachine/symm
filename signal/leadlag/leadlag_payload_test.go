package leadlag

func inefficientLagPayload() []float64 {
	return []float64{0, 100, 1, 1, 0, 1, 8, 0.9, 0, 0, 20}
}

func syncDriftPayload() []float64 {
	return []float64{0, 100, 1, 1, 0, 1, 2, 0.9, 1, 0.9, 20}
}

func decoupledMovePayload() []float64 {
	return []float64{0, 100, 1, 0, 0.5, 0, 0, 0, 1, 0.01, 20}
}

func anchorStallPayload() []float64 {
	return []float64{1, 50000, 1, 0, 0.8, 0, 0, 0, 0, 0, 0}
}
