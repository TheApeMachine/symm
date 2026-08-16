package equation

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*Exhaustion calculates exhaustion based on declining relative lift and price rejection.
Its map carries "priorRVOL", "rvol", "rejection", "moveScale", producing "result".*/
type Exhaustion struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Exhaustion)(nil)

/*NewExhaustion returns an Exhaustion primitive.*/
func NewExhaustion(initial types.Input[types.Map[string, types.Value[float64]]]) *Exhaustion {
	return &Exhaustion{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*Write stages the exhaustion map.*/
func (ex *Exhaustion) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		ex.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"exhaustion: input is nil",
			nil,
		))

		return
	}

	ex.next.Write(input)
	ex.err = nil
}

/*Read computes the exhaustion result = (max(0, priorRVOL - rvol) / priorRVOL) * ignitionSquash(rejection, moveScale).*/
func (ex *Exhaustion) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := ex.next.Read()

	if in.Error() != "" {
		ex.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return ex.next
	}

	mapping := in.Project().Read()
	priorRVOLVal, hasPriorRVOL := mapping.Get("priorRVOL")
	rvolVal, hasRvol := mapping.Get("rvol")
	rejectionVal, hasRejection := mapping.Get("rejection")
	moveScaleVal, hasMoveScale := mapping.Get("moveScale")

	if !hasPriorRVOL || !hasRvol || !hasRejection || !hasMoveScale {
		ex.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"exhaustion: missing one or more fields",
			nil,
		))

		return ex.next
	}

	priorRVOL := priorRVOLVal.Read()
	rvol := rvolVal.Read()
	rejection := rejectionVal.Read()
	moveScale := moveScaleVal.Read()

	if priorRVOL <= 0 || rejection <= 0 || moveScale <= 0 {
		mapping.Put("result", types.NewValue(0.0))
		ex.next.Write(types.NewInput(types.NewValue(mapping)))
		ex.err = nil

		return ex.next
	}

	squashVal := rejection / (moveScale + rejection)
	exhaustion := (math.Max(0, priorRVOL-rvol) / priorRVOL) * squashVal

	mapping.Put("result", types.NewValue(exhaustion))

	ex.next.Write(types.NewInput(types.NewValue(mapping)))
	ex.err = nil

	return ex.next
}

/*Project returns the current projected map.*/
func (ex *Exhaustion) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return ex.next.Project()
}

/*Error reports any execution error.*/
func (ex *Exhaustion) Error() string {
	if ex.err != nil {
		return ex.err.Error()
	}

	return ex.next.Error()
}

/*Close releases internal state.*/
func (ex *Exhaustion) Close() error {
	if err := ex.initial.Close(); err != nil {
		return err
	}

	if err := ex.next.Close(); err != nil {
		return err
	}

	ex.err = nil

	return nil
}