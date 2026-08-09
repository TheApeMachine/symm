package types

/*
Symbol is an alternative grouping of measurements, and is used in logic that
legitimately needs to fan out measurements by symbol, rather than by source.
It should be kept extremely simple, and lean, and is not an invitation to
start adding additional complexity beyond what is truly earned. Symbols
carry their own readiness state, which is important for the resonance solver,
which needs a stable measurement set to train on.
*/
type Symbol struct {
	Readiness
	Measurements []*Measurement `json:"measurements,omitempty"`
}
