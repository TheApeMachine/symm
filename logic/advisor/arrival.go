package advisor

import (
	"github.com/theapemachine/symm/types"
)

/*
ArrivalBindings declares the Hawkes arrival-process state: empirical arrival
rates, conditional (pre-arrival) intensities, background rates, and excitation
fractions. These describe event frequency and endogenous excitation; they are
not price forecasts. Hawkes emits side-qualified excitation fractions only
(excitation_fraction:buy / :sell); there is no unqualified excitation_fraction
in the signal output, so the advisor composes the side-qualified facts.
*/
func ArrivalBindings() []MetricBinding {
	return []MetricBinding{
		NewMetricBinding("hawkes", "arrival_rate", "advisor/arrival/arrival_rate"),
		NewMetricBinding("hawkes", "arrival_rate:buy", "advisor/arrival/arrival_rate_buy"),
		NewMetricBinding("hawkes", "arrival_rate:sell", "advisor/arrival/arrival_rate_sell"),
		NewMetricBinding("hawkes", "conditional_intensity", "advisor/arrival/conditional_intensity"),
		NewMetricBinding("hawkes", "conditional_intensity:buy", "advisor/arrival/conditional_intensity_buy"),
		NewMetricBinding("hawkes", "conditional_intensity:sell", "advisor/arrival/conditional_intensity_sell"),
		NewMetricBinding("hawkes", "background_rate", "advisor/arrival/background_rate"),
		NewMetricBinding("hawkes", "background_rate:buy", "advisor/arrival/background_rate_buy"),
		NewMetricBinding("hawkes", "background_rate:sell", "advisor/arrival/background_rate_sell"),
		NewMetricBinding("hawkes", "excitation_fraction:buy", "advisor/arrival/excitation_fraction_buy"),
		NewMetricBinding("hawkes", "excitation_fraction:sell", "advisor/arrival/excitation_fraction_sell"),
	}
}

/*
NewArrivalAdvisor constructs the KindArrival Advisor: event frequency and
endogenous excitation state, surfaced from the Hawkes signal's own readings.
*/
func NewArrivalAdvisor(name string) *Advisor {
	return newLatestFactsAdvisor(name, types.KindArrival, ArrivalBindings())
}
