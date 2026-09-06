package correlation

/*
Point pairs a value with the coordinate it was measured at.

A search reports two things at once — where it looked and what it found —
and a Primitive yields one value, so the two travel together. It is
deliberately unnamed for any one use: a correlation against a shift, a peer
against its weight, and a reading against its time are the same shape.
*/
type Point struct {
	X float64
	Y float64
}
