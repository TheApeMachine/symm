package hawkes

/*
NewIntegralSupport composes the exponential compensator numerator per side.
Divide by beta in the compensator, not here.
*/
func NewIntegralSupport() *Integral {
	return NewIntegral(func(beta, lower, upper float64) float64 {
		return kernel(beta, lower) - kernel(beta, upper)
	})
}
