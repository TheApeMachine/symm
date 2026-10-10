package core

import "math"

/*
LogModulus compresses a signed quantity whose magnitude is multiplicative (a
rate or velocity over a venue interval spans orders of magnitude) onto a log
scale without losing its sign: y = sign(x) * ln(1 + |x|/u), where u is the
geometric mean magnitude of the nonzero values before x. The unit comes from
the stream, not a constant, the transform keeps order and sign, and zero maps
to zero. A plain logarithm cannot do this: it is undefined at zero and for
negative values, and log|x| would turn a small move into a large negative one.
*/
type LogModulus struct {
	count float64
	mean  float64
}

/*
Step returns x on the log-modulus scale against the magnitudes before it, then
incorporates |x|. It is undefined until a nonzero magnitude has been seen.
*/
func (modulus *LogModulus) Step(x float64) (float64, bool) {
	var compressed float64
	defined := modulus.count > 0

	if defined && x != 0 {
		compressed = math.Copysign(math.Log1p(math.Abs(x)/math.Exp(modulus.mean)), x)
	}

	if x != 0 {
		modulus.count++
		modulus.mean += (math.Log(math.Abs(x)) - modulus.mean) / modulus.count
	}

	return compressed, defined
}
