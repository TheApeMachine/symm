/*
Package correlation estimates dependence between paths that were sampled
asynchronously, as two order books are: neither path is resampled onto an
invented clock, and no fixed window, lag range, or resolution is configured.

Everything here is a core.Primitive. A run of observations, a run of return
intervals, and a correlation profile all travel as ordinary values through
Next, so a path composes with arithmetic and calculus exactly as a scalar
does, and a consumer never learns whether it was handed a carrier or a graph.
*/
package correlation

/*
Observation is one point on a path: what was seen, and when it was seen.

The timestamp is carried with the value rather than implied by position,
because two paths that were never sampled together can only be related
through the times they were each sampled at.
*/
type Observation struct {
	Nanos int64
	Value float64
}
