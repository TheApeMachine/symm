package statistic

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Likelihood computes log-likelihood differentials between Hawkes model
and homogeneous Poisson baseline: LL_delta = LL_hawkes - LL_poisson.
*/
type Likelihood struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Likelihood)(nil)

/*
NewLikelihood creates a Likelihood analysis primitive.
*/
func NewLikelihood(
	initial types.Input[types.Map[string, types.Value[float64]]],
) *Likelihood {
	return &Likelihood{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*
Write stages the log likelihood parameters.
*/
func (likelihood *Likelihood) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		likelihood.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"likelihood: input is nil",
			nil,
		))

		return
	}

	likelihood.next.Write(input)
	likelihood.err = nil
}

/*
Read computes log likelihood deltas.
*/
func (likelihood *Likelihood) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := likelihood.next.Read()

	if in.Error() != "" {
		likelihood.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return likelihood.next
	}

	params := in.Project().Read()
	llHawkes := likelihood.number(params, "ll_hawkes")
	llPoisson := likelihood.number(params, "ll_poisson")
	llSelf := likelihood.number(params, "ll_self")

	deltaPoisson := llHawkes - llPoisson
	deltaSelf := llHawkes - llSelf

	if math.IsNaN(deltaPoisson) || math.IsInf(deltaPoisson, 0) {
		deltaPoisson = 0
	}

	if math.IsNaN(deltaSelf) || math.IsInf(deltaSelf, 0) {
		deltaSelf = 0
	}

	params.Put("ll_delta_poisson", types.NewValue(deltaPoisson))
	params.Put("ll_delta_self", types.NewValue(deltaSelf))

	likelihood.next.Write(types.NewInput(types.NewValue(params)))
	likelihood.err = nil

	return likelihood.next
}

/*
Project returns the computed likelihood deltas.
*/
func (likelihood *Likelihood) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return likelihood.next.Project()
}

/*
Error reports an error.
*/
func (likelihood *Likelihood) Error() string {
	if likelihood.err != nil {
		return likelihood.err.Error()
	}

	return likelihood.next.Error()
}

/*
Close releases the input state.
*/
func (likelihood *Likelihood) Close() error {
	if err := likelihood.initial.Close(); err != nil {
		return err
	}

	if err := likelihood.next.Close(); err != nil {
		return err
	}

	likelihood.err = nil

	return nil
}

func (likelihood *Likelihood) number(
	params types.Map[string, types.Value[float64]],
	name string,
) float64 {
	val, found := params.Get(name)

	if !found {
		return 0
	}

	return val.Read()
}
