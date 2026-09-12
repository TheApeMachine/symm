package temporal

/*
Price is a scalar value at its nanosecond timestamp coordinate.
*/
type Price struct {
	At    int64
	Value float64
}
