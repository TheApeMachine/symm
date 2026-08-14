package algo

import (
	"time"

	"github.com/theapemachine/errnie"
	excitation "github.com/theapemachine/symm/nomagique/algo/hawkes"
	"github.com/theapemachine/symm/nomagique/types"
)

type hawkesState = types.Pair[string, types.Map[string, types.Value[float64]]]

/*
Hawkes is the bivariate exponential estimator. Write stages a symbol and a
named map; a non-zero side appends one arrival. Read measures the staged
stream and writes the fitted state back onto the same pair.
*/
type Hawkes struct {
	Input   types.Input[hawkesState]
	Output  types.Output[hawkesState]
	sample  *excitation.Sample
	process *excitation.Process
	stream  excitation.Input
	reason  string
	err     error
}

var _ types.IO[hawkesState] = (*Hawkes)(nil)

func NewHawkes() *Hawkes {
	return &Hawkes{
		Input:   types.NewInput[hawkesState](),
		Output:  types.NewOutput[hawkesState](),
		sample:  excitation.NewSample(),
		process: excitation.NewProcess(),
	}
}

func (hawkes *Hawkes) Write(input types.Input[hawkesState]) {
	hawkes.Input.Write(input)
	hawkes.err = nil
	pair := input.Project().Read()
	collected := pair.Value
	side := hawkes.number(collected, "side")

	if side == 0 {
		return
	}

	mark := "buy"

	if side < 0 {
		mark = "sell"
	}

	sampled, _, err := hawkes.sample.MeasureArrival(excitation.TradeInput{
		Symbol:    pair.Key,
		Side:      mark,
		Timestamp: hawkes.stamp(collected),
	})

	if err != nil {
		hawkes.err = err

		return
	}

	hawkes.stream = sampled
}

func (hawkes *Hawkes) Read() types.Output[hawkesState] {
	if hawkes.stream.Symbol == "" {
		hawkes.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"hawkes: arrival stream has not been written",
			nil,
		))

		return hawkes.Output
	}

	outcome, ready, err := hawkes.process.Measure(hawkes.stream)

	if err != nil {
		hawkes.err = err

		return hawkes.Output
	}

	hawkes.reason = outcome.Readiness.Reason
	hawkes.Output.Write(&types.InputValue[hawkesState]{
		Value: types.NewValue(types.NewPair(hawkes.stream.Symbol, hawkes.measured(outcome, ready))),
	})
	hawkes.err = nil

	return hawkes.Output
}

func (hawkes *Hawkes) Project() types.Value[hawkesState] {
	return hawkes.Output.Project()
}

func (hawkes *Hawkes) Error() string {
	if hawkes.err != nil {
		return hawkes.err.Error()
	}

	return hawkes.Output.Error()
}

func (hawkes *Hawkes) Close() error {
	if err := hawkes.Input.Close(); err != nil {
		return err
	}

	return hawkes.Output.Close()
}

func (hawkes *Hawkes) AwaitFit() bool {
	return hawkes.process.AwaitFit(hawkes.stream.Symbol)
}

func (hawkes *Hawkes) Reason() string {
	return hawkes.reason
}

func (hawkes *Hawkes) measured(outcome excitation.Out, ready bool) types.Map[string, types.Value[float64]] {
	collected := types.NewMap[string, types.Value[float64]]()
	hawkes.put(collected, "ready", hawkes.flag(ready))
	hawkes.put(collected, "observation", hawkes.flag(outcome.Readiness.Observation))
	hawkes.put(collected, "intensity", hawkes.flag(outcome.Readiness.Intensity))
	hawkes.put(collected, "hawkes_fit", hawkes.flag(outcome.Readiness.HawkesFit))
	hawkes.put(collected, "model_updated", hawkes.flag(outcome.Readiness.ModelUpdated))
	hawkes.put(collected, "forecast", hawkes.flag(outcome.Readiness.Forecast))
	hawkes.put(collected, "event_count", float64(outcome.EventCount))
	hawkes.put(collected, "alpha_event_count", float64(outcome.AlphaEventCount))
	hawkes.put(collected, "beta_event_count", float64(outcome.BetaEventCount))
	hawkes.put(collected, "alpha_arrival_rate", outcome.AlphaArrivalRate)
	hawkes.put(collected, "beta_arrival_rate", outcome.BetaArrivalRate)
	hawkes.put(collected, "maturity", outcome.Maturity)
	hawkes.put(collected, "minimum_fit_events", float64(outcome.MinimumFitEvents))
	hawkes.put(collected, "lambda_buy", outcome.Fit.IntensityX)
	hawkes.put(collected, "lambda_sell", outcome.Fit.IntensityY)
	hawkes.put(collected, "mu_buy", outcome.Fit.MuX)
	hawkes.put(collected, "mu_sell", outcome.Fit.MuY)
	hawkes.put(collected, "alpha_bb", outcome.Fit.AlphaXX)
	hawkes.put(collected, "alpha_bs", outcome.Fit.AlphaXY)
	hawkes.put(collected, "alpha_sb", outcome.Fit.AlphaYX)
	hawkes.put(collected, "alpha_ss", outcome.Fit.AlphaYY)
	hawkes.put(collected, "beta", outcome.Fit.Beta)
	hawkes.put(collected, "spectral_radius", outcome.Fit.SpectralRadius)
	hawkes.put(collected, "offspring_bb", outcome.ImmediateAlphaOffspring)
	hawkes.put(collected, "offspring_bs", outcome.ImmediateBetaOffspring)
	hawkes.put(collected, "descendants_buy", outcome.TotalAlphaDescendants)
	hawkes.put(collected, "descendants_sell", outcome.TotalBetaDescendants)
	hawkes.put(collected, "ll_delta_poisson", outcome.HawkesPoissonLogLikelihoodDelta)
	hawkes.put(collected, "ll_delta_self", outcome.CrossSelfLogLikelihoodDelta)
	hawkes.put(collected, "fit_valid", hawkes.flag(outcome.Fit.Valid()))
	hawkes.put(collected, "horizon_seconds", outcome.Horizon.Seconds())
	hawkes.put(collected, "observed_from_sec", float64(outcome.ObservedFrom.Unix()))
	hawkes.put(collected, "observed_from_nsec", float64(outcome.ObservedFrom.Nanosecond()))
	hawkes.put(collected, "observed_at_sec", float64(outcome.ObservedAt.Unix()))
	hawkes.put(collected, "observed_at_nsec", float64(outcome.ObservedAt.Nanosecond()))
	hawkes.put(collected, "fit_at_sec", float64(outcome.FitObservedAt.Unix()))
	hawkes.put(collected, "fit_at_nsec", float64(outcome.FitObservedAt.Nanosecond()))

	if outcome.Fit.Beta > 0 {
		hawkes.put(collected, "fold", 1/outcome.Fit.Beta)
	}

	return collected
}

func (hawkes *Hawkes) stamp(collected types.Map[string, types.Value[float64]]) time.Time {
	_, hasSeconds := collected.Get("unix_sec")
	_, hasNanos := collected.Get("unix_nsec")

	if !hasSeconds && !hasNanos {
		return time.Time{}
	}

	return time.Unix(
		int64(hawkes.number(collected, "unix_sec")),
		int64(hawkes.number(collected, "unix_nsec")),
	).UTC()
}

func (hawkes *Hawkes) number(
	collected types.Map[string, types.Value[float64]],
	name string,
) float64 {
	value, found := collected.Get(name)

	if !found {
		return 0
	}

	return value.Read()
}

func (hawkes *Hawkes) put(
	collected types.Map[string, types.Value[float64]],
	name string,
	value float64,
) {
	collected.Put(name, types.NewValue(value))
}

func (hawkes *Hawkes) flag(yes bool) float64 {
	if yes {
		return 1
	}

	return 0
}
