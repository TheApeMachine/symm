package types

/*
Signal is ONLY a composed nomagique pipeline. A signal owns no scheduling
state and no per-symbol bookkeeping: it exposes one primitive that maps an
encoded market row Frame to a measurement Frame, and the runner (the trader)
owns the per-symbol streams, the queue drains, and the emit provenance.

The three remaining members are boundary adapters, not numeric logic:
Rows yields the raw market rows the pipeline consumes, Encode prepares one raw
row for the pipeline, and Emit projects the pipeline's output Frame into the
shared Measurement shape, taking the symbol name from the runner because a
numeric Frame cannot carry it.
*/
type Signal interface {
	Name() string
	Error() error
	Type() SourceType
	Close() error
}

/*
CohortSignal owns a cross-sectional calculation. The measurement scheduler calls
it once for the complete dirty symbol set carried by one transport arrival, so
all rows are ingested before any member of that cohort is scored.
*/
type CohortSignal interface {
	Signal
}
