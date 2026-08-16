package statistic

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Branching calculates the branching matrix, spectral radius, immediate offspring,
and total descendants for a bivariate excitation kernel with decay beta.
*/
type Branching struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Branching)(nil)

/*
NewBranching creates a Branching analysis primitive.
*/
func NewBranching(
	initial types.Input[types.Map[string, types.Value[float64]]],
) *Branching {
	return &Branching{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*
Write stages the parameter map containing alpha_aa, alpha_ab, alpha_ba, alpha_bb, beta.
*/
func (branching *Branching) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		branching.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"branching: input is nil",
			nil,
		))

		return
	}

	branching.next.Write(input)
	branching.err = nil
}

/*
Read computes the spectral radius, offspring, and descendants.
*/
func (branching *Branching) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := branching.next.Read()

	if in.Error() != "" {
		branching.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return branching.next
	}

	params := in.Project().Read()
	beta := branching.number(params, "beta")

	if beta <= 0 {
		branching.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"branching: beta must be positive",
			nil,
		))

		return branching.next
	}

	alphaAA := branching.number(params, "alpha_aa") / beta
	alphaAB := branching.number(params, "alpha_ab") / beta
	alphaBA := branching.number(params, "alpha_ba") / beta
	alphaBB := branching.number(params, "alpha_bb") / beta

	trace := alphaAA + alphaBB
	det := (alphaAA * alphaBB) - (alphaAB * alphaBA)
	discriminant := (trace * trace) - (4 * det)

	if discriminant < 0 {
		discriminant = 0
	}

	spectralRadius := (trace + math.Sqrt(discriminant)) / 2.0

	detIminusGamma := (1-alphaAA)*(1-alphaBB) - (alphaAB * alphaBA)
	descendantsAlpha := 1.0
	descendantsBeta := 1.0

	if detIminusGamma > 1e-9 {
		descendantsAlpha = ((1 - alphaBB + alphaAB) / detIminusGamma)
		descendantsBeta = ((alphaBA + 1 - alphaAA) / detIminusGamma)
	}

	params.Put("spectral_radius", types.NewValue(spectralRadius))
	params.Put("offspring_aa", types.NewValue(alphaAA))
	params.Put("offspring_ab", types.NewValue(alphaAB))
	params.Put("offspring_ba", types.NewValue(alphaBA))
	params.Put("offspring_bb", types.NewValue(alphaBB))
	params.Put("descendants_alpha", types.NewValue(descendantsAlpha))
	params.Put("descendants_beta", types.NewValue(descendantsBeta))

	branching.next.Write(types.NewInput(types.NewValue(params)))
	branching.err = nil

	return branching.next
}

/*
Project returns the computed branching metrics.
*/
func (branching *Branching) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return branching.next.Project()
}

/*
Error reports an error.
*/
func (branching *Branching) Error() string {
	if branching.err != nil {
		return branching.err.Error()
	}

	return branching.next.Error()
}

/*
Close releases the input state.
*/
func (branching *Branching) Close() error {
	if err := branching.initial.Close(); err != nil {
		return err
	}

	if err := branching.next.Close(); err != nil {
		return err
	}

	branching.err = nil

	return nil
}

func (branching *Branching) number(
	params types.Map[string, types.Value[float64]],
	name string,
) float64 {
	val, found := params.Get(name)

	if !found {
		return 0
	}

	return val.Read()
}
