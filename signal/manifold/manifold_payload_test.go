package manifold

func herdManifoldPayload() []float64 {
	return []float64{1, 0.9, 10, 2, 50000}
}

func shockManifoldPayload() []float64 {
	return []float64{20, 0.5, 1, 2, 50000}
}

func driftManifoldPayload() []float64 {
	return []float64{2, 0.3, 15, 1, 50000}
}

func noiseManifoldPayload() []float64 {
	return []float64{0.1, 0.1, 0.5, 10, 50000}
}
