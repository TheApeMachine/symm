package adaptive

/*
Baseline tracks an adaptive statistical baseline reference level.
Outputs the shock/residual relative to baseline, or the running baseline center.
Zero magic numbers: center derived online via Welford moments.
*/
type Baseline struct {
	Engine WelfordEngine
}

func (baseline *Baseline) Step(number Number) Number {
	mean, _ := baseline.Engine.Update(float64(number))

	return Number(float64(number) - mean)
}
