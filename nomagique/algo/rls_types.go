package algo

/*
RLSState is the posterior and the query design used to forecast.
*/
type RLSState struct {
	Beta         []float64
	Design       []float64
	Root         [][]float64
	NoiseShape   float64
	NoiseScale   float64
	Observations float64
}

/*
RLSForecast is the projection of a posterior through a design.
*/
type RLSForecast struct {
	RLSState
	Prediction         float64
	Factor             []float64
	Scale              float64
	DegreesOfFreedom   float64
	PredictiveVariance float64
	Ready              bool
}
