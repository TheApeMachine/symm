package algo

import (
	"github.com/theapemachine/symm/nomagique/statistic/hawkes"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Hawkes is a composite Primitive assembled entirely from atomic units in
nomagique/statistic/hawkes. Composition order is the causal contract in
signal/hawkes/README.md section 5.3: every primitive up through Compensator
reads only the model fitted BEFORE the current event and the arrival path
retained BEFORE it (ArrivalPath has not run yet), so the current event can
neither excite nor refit the model that judges it. Fit and ArrivalPath run
last, in that order, so a converged refit and the event's own retention both
become visible only starting with the NEXT event.

	empirical counts/rates, mark composition, From/At/Maturity (Accounting)
	-> pre-arrival conditional intensity λ(t⁻) (ConditionalIntensity)
	-> exact point-process likelihood vs. Poisson/self-only (Likelihood)
	-> branching matrix, spectral radius, descendants (Branching)
	-> excess intensity, amplitude, decay/timescale (Excitation)
	-> compensator, innovations, SNR (Compensator)
	-> data-derived MLE refit of the model for subsequent events (Fit)
	-> retain the current event in the bounded arrival path (ArrivalPath)
*/
func Hawkes() types.Primitive {
	return types.Pipe(
		hawkes.Accounting,
		hawkes.ConditionalIntensity,
		hawkes.Likelihood,
		hawkes.Branching,
		hawkes.Excitation,
		hawkes.Compensator,
		hawkes.Fit,
		hawkes.ArrivalPath,
	)
}
