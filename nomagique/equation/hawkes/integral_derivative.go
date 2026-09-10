package hawkes

/*
NewIntegralDerivative is the derivative of the integral-support numerator
with respect to beta, retaining both boundary-age terms.
*/
func NewIntegralDerivative() *Integral {
	return NewIntegral(func(beta, lower, upper float64) float64 {
		return upper*kernel(beta, upper) - lower*kernel(beta, lower)
	})
}
