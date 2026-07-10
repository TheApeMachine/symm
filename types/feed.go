package types

/*
Feed drains queued market frames and returns measurements from composed signals.
*/
type Feed interface {
	StatusReporter
	On([]byte)
	Measure() ([]*Measurement, error)
}
