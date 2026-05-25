package adaptive

/*
BlendEMA moves current toward observation by alpha in (0, 1].
*/
func BlendEMA(current, observation, alpha float64) float64 {
	return current + alpha*(observation-current)
}
