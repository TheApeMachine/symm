package correlation

/*
Interval is one return realized over the half-open span (From, To].

A return is a statement about a stretch of time rather than about an instant,
and which stretch it covers is what decides whether it overlaps a return on
another path. Carrying the span with the value is what lets dependence be
estimated without either path being resampled.
*/
type Interval struct {
	From  int64
	To    int64
	Value float64
}
