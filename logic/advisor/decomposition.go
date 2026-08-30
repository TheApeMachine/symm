package advisor

import (
	nmtypes "github.com/theapemachine/symm/nomagique/types"
	"github.com/theapemachine/symm/types"
)

/*
DecompositionBindings declares the two CVD-internal quantities that jointly
separate the mechanisms a single raw "activity" number conflates, both from the
SAME retained CVD interval so they describe one coherent trade population:

  - cvd/trade_rate — event frequency: trades per second (N/Δt over CVD's own
    retained path).
  - cvd/mean_trade_notional — mean economic event size: gross notional per
    trade (G/N over the same retained path).

CVD already publishes mean_trade_notional (see signal/cvd/README.md §8.6)
precisely to distinguish many-small from few-large trades. It is computed by
the CVD signal from its own retained interval, so the numerator (gross
notional) and denominator (trade count) describe the SAME trade population. The
DECOMPOSES relation's own normative meaning is "arrival intensity × mean trade
notional separates event frequency from economic event size" — it does not ask
any consumer to re-derive mean size by dividing CVD's gross-notional rate by
Hawkes' arrival rate. Hawkes and CVD may retain different observation horizons
and count different event populations, so such a cross-signal division would be
dimensionally lawful (quote/sec ÷ events/sec = quote/event) yet economically
incoherent: it could equate a CVD flow over [10,100] with a Hawkes rate over
[99,100]. The canonical quantity is mean_trade_notional, and the frequency that
shares its population is CVD's own trade_rate. No Hawkes metric is consumed
here.
*/
func DecompositionBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("cvd", "trade_rate", "advisor/decomposition/frequency"),
		NewMetricBinding("cvd", "mean_trade_notional", "advisor/decomposition/mean_size"),
	}
}

/*
DecompositionPipeline relays each bound metric's raw value into its own slot
(gated on Fresh). There is no derived division: mean_trade_notional IS the
canonical "many-small vs few-large" discriminator the CVD signal already
computed from its own coherent interval, and trade_rate is the frequency over
that same interval. The two facts are carried together with their own
provenance, so a consumer reads "N trades/sec with mean size G/N" — an honest
decomposition of which mechanism (frequency vs size) is driving activity —
without any third quantity fabricated from mismatched Hawkes/CVD horizons.
*/
func DecompositionPipeline(bindings []MetricBinding) nmtypes.Primitive {
	frequency := bindings[0]
	meanSize := bindings[1]

	return nmtypes.Pipe(
		nmtypes.ForkStrict(jointFact(frequency), jointFact(meanSize)),
		scrubFresh(bindings),
	)
}

/*
DecompositionOutputs declares the two facts: event frequency (trade_rate) and
mean economic event size (mean_trade_notional), each borrowing its own bound
metric's provenance. Nothing is derived here, because the canonical
mean-trade-notional quantity already exists in the CVD signal and must not be
duplicated by an advisor.
*/
func DecompositionOutputs(bindings []MetricBinding) []Output {
	return []Output{
		NewMetricOutput(bindings[0].Series.ValueSymbol, bindings[0]), // trade_rate
		NewMetricOutput(bindings[1].Series.ValueSymbol, bindings[1]), // mean_trade_notional
	}
}

/*
NewDecompositionAdvisor constructs the decomposition Advisor instance. It
answers the named question: is the current executed activity many-small trades
or few-large trades — by surfacing CVD's canonical mean trade notional and its
own coherent trade rate, never by dividing mismatched cross-signal horizons.
*/
func NewDecompositionAdvisor(name string) *Advisor {
	bindings := DecompositionBindings()

	return NewAdvisor(
		name,
		types.KindDecomposition,
		DecompositionPipeline(bindings),
		bindings,
		DecompositionOutputs(bindings),
	)
}
