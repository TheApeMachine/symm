package cmd

/*
TuneFitness is the scalar score symm tune maximizes.
It rewards wallet PnL and penalizes profitable entries blocked by gates.
*/
func TuneFitness(scoreEUR, missedForwardEUR float64) float64 {
	return scoreEUR - missedForwardEUR
}
