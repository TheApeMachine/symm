/*
Package relation measures directed temporal predictive contribution between
Measurement coordinates.

A Relation answers one question:

	Did knowing Source history improve prediction of Target beyond Target's
	own history and explicitly supplied Controls?

Relation measures predictive Influence. It does not prove physical causality,
does not decide actions, does not create market categories, and never deletes
measurements. Feature selection is query-local; the full observational
coordinate history remains available.

Everything is a core.Primitive over the unsafe.Pointer wire, and nothing
else: ObservationStore retains one bounded chronological ring per coordinate,
Project splits Measurements into store batches, Planner compiles one plan
into candidate adapters, Align walks lagged series, and Influence measures
prequential predictive contribution and publishes it on the adapter.

A coordinate is identified by one key whose fields are joined with "|":

	symbol|source|metric|side|peer|unit|timescale|epoch

Every field participates in identity, so incompatible epochs, units, or
timescales are never mixed. A selector is [3]string{source, metric, side};
an empty field is a wildcard for that component.

An observation is the pair {at, raw}: at is the as-of instant in Unix
nanoseconds, raw the signed metric value. Series are flattened
{at0, raw0, at1, raw1, ...}. Lags are nanoseconds.
*/
package relation

/*
Fit states published as "status" by Influence. Invalid is not zero: every
failure state is distinct and observable.
*/
const (
	FitOK = iota
	FitNoSourceHistory
	FitNoTargetHistory
	FitControlUnavailable
	FitNoPositiveLag
	FitNoAlignedRows
	FitInsufficientSupport
	FitRankDeficient
	FitResidualVarianceUnavailable
)
