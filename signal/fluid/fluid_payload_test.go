package fluid

func laminarFluidPayload() []float64 {
	return []float64{0.5, 0.01, 0.8, 1, 1, 2, 4, 0, 0.05, 0, 100, 2, 0.01, 1000}
}

func turbulentFluidPayload() []float64 {
	return []float64{8, 0.2, 0.5, 1, 1, 2, 4, 1, 0.1, 0, 100, 2, 0.01, 1000}
}

func inertialFluidPayload() []float64 {
	return []float64{1, 10, 2, 1, 1, 20, 4, 0, 11, 0, 100, 2, 0.01, 1000}
}

func viscousFluidPayload() []float64 {
	return []float64{1, 3, 0.3, 1, 1, 10, 4, 0, 0.05, 15, 100, 2, 0.01, 1000}
}
