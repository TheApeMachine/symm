package algo

import (
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Hawkes is a composite Primitive assembled entirely from atomic units in
nomagique/statistic/hawkes. Composition order is the causal contract in
signal/hawkes/README.md section 5.3:

	1. evaluate pre-arrival intensity from history/parameters before this event
	2. evaluate likelihood and compensator contribution
	3. emit the measurement
	4. incorporate the event into process state
	5. refit/update model parameters for subsequent observations

Steps 1-3 (Accounting through Compensator) read only the model fitted BEFORE
the current event and the arrival path retained BEFORE it — ArrivalPath has
not run yet, so the current event can neither excite nor refit the model
that judges it. ArrivalPath then performs step 4 (incorporate), and Fit runs
LAST, performing step 5 (refit) from history that now INCLUDES the current
event — the model Fit produces here first takes effect for the event AFTER
this one, never this one.

	empirical counts/rates, mark composition, From/At/Maturity (Accounting)
	-> pre-arrival conditional intensity λ(t⁻) (ConditionalIntensity)
	-> exact point-process likelihood vs. Poisson/self-only (Likelihood)
	-> branching matrix, spectral radius, descendants (Branching)
	-> excess intensity, amplitude, decay/timescale (Excitation)
	-> compensator, innovations, SNR (Compensator)
	-> retain the current event in the bounded arrival path (ArrivalPath)
	-> data-derived MLE refit of the model for subsequent events (Fit)
*/
func Hawkes() types.Primitive {
	return types.Pipe(
		hawkes.Accounting,
		hawkes.ConditionalIntensity,
		hawkes.Likelihood,
		hawkes.Branching,
		hawkes.Excitation,
		hawkes.Compensator,
		hawkes.ArrivalPath,
		hawkes.Fit,
	)
}
