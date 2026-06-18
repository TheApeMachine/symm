package depthflow

func bookThinningPayload() []float64 {
	weightedHistory := []float64{0.85, 0.86, 0.87, 0.88, 0.89, 0.9, 0.91, 0.92}
	level1History := []float64{0.8, 0.81, 0.82, 0.83, 0.84, 0.85, 0.86, 0.87}
	flatHistory := []float64{0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02}

	samples := []float64{
		0.9, 0.85, 0.02, 1, 100, 2, 50, 0.3,
		float64(len(weightedHistory)),
		float64(len(level1History)),
		float64(len(flatHistory)),
	}
	samples = append(samples, weightedHistory...)
	samples = append(samples, level1History...)
	samples = append(samples, flatHistory...)

	return samples
}

func denseNeutralityPayload() []float64 {
	weightedHistory := []float64{0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02}
	level1History := []float64{0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02}
	flatHistory := []float64{0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02, 0.02}

	samples := []float64{
		0.005, 0.005, 0.005, 0, 100, 2, 50, 0.01,
		float64(len(weightedHistory)),
		float64(len(level1History)),
		float64(len(flatHistory)),
	}
	samples = append(samples, weightedHistory...)
	samples = append(samples, level1History...)
	samples = append(samples, flatHistory...)

	return samples
}

func loadedImbalancePayload() []float64 {
	weightedHistory := []float64{0.8, 0.82, 0.84, 0.86, 0.88, 0.9, 0.92, 0.94}
	level1History := []float64{0.7, 0.72, 0.74, 0.76, 0.78, 0.8, 0.82, 0.84}
	flatHistory := []float64{0.75, 0.76, 0.77, 0.78, 0.79, 0.8, 0.81, 0.82}

	samples := []float64{
		0.9, 0.85, 0.8, 0, 100, 2, 50, 0.8,
		float64(len(weightedHistory)),
		float64(len(level1History)),
		float64(len(flatHistory)),
	}
	samples = append(samples, weightedHistory...)
	samples = append(samples, level1History...)
	samples = append(samples, flatHistory...)

	return samples
}

func spoofTrapPayload() []float64 {
	weightedHistory := []float64{0.85, 0.86, 0.87, 0.88, 0.89, 0.9, 0.91, 0.92}
	level1History := []float64{-0.3, -0.32, -0.34, -0.36, -0.38, -0.4, -0.42, -0.44}
	flatHistory := []float64{0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5, 0.5}

	samples := []float64{
		0.9, -0.35, 0.5, 1, 50, 2, 40, -0.2,
		float64(len(weightedHistory)),
		float64(len(level1History)),
		float64(len(flatHistory)),
	}
	samples = append(samples, weightedHistory...)
	samples = append(samples, level1History...)
	samples = append(samples, flatHistory...)

	return samples
}
