package hawkes

import (
	"time"

	"github.com/theapemachine/nomagique/hawkes"
)

/*
Out contains the numerical state of a bivariate exponential Hawkes model.
It carries no market category or trading gate because those interpretations
belong to the consuming logic and strategy layers.
*/
type Out struct {
	Fit                             hawkes.BivariateFit
	ObservedFrom                    time.Time
	ObservedAt                      time.Time
	Horizon                         time.Duration
	FitObservedFrom                 time.Time
	FitObservedAt                   time.Time
	HawkesPoissonLogLikelihoodDelta float64
	CrossSelfLogLikelihoodDelta     float64
	ImmediateAlphaOffspring         float64
	ImmediateBetaOffspring          float64
	TotalAlphaDescendants           float64
	TotalBetaDescendants            float64
	EventCount                      int
	AlphaEventCount                 int
	BetaEventCount                  int
	AlphaArrivalRate                float64
	BetaArrivalRate                 float64
	MinimumFitEvents                int
	Maturity                        float64
	Readiness                       Readiness
}
