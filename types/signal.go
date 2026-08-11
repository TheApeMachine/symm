package types

/*
Signal measures market rows from explicit transport subscriptions and publishes
shared Thesis updates downstream.
*/
type Signal interface {
	Name() string
	Type() SourceType
	Measure(thesis *Thesis) []*Measurement
	Close() error
}
