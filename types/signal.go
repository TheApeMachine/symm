package types

import "iter"

/*
Signal measures market rows from explicit transport subscriptions and publishes
shared Thesis updates downstream.
*/
type Signal interface {
	Name() string
	Type() SourceType
	Measure(*Symbol, ...int64) iter.Seq[*Measurement]
	Close() error
}

/*
CohortSignal owns a cross-sectional calculation. The measurement scheduler calls
it once for the complete dirty symbol set carried by one transport arrival, so
all rows are ingested before any member of that cohort is scored.
*/
type CohortSignal interface {
	Signal
	MeasureCohort([]*Symbol, ...int64) iter.Seq[*Measurement]
}
