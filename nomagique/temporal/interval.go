package temporal

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

/*
Interval computes elapsed delta between sequential timestamps.
Its map carries "timestamp", "previous", "delta", and "has_seen".
*/
type Interval struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

var _ types.IO[types.Map[string, types.Value[float64]]] = (*Interval)(nil)

/*
NewInterval creates an Interval tracking primitive.
*/
func NewInterval(
	initial types.Input[types.Map[string, types.Value[float64]]],
) *Interval {
	return &Interval{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

/*
Write stages the timestamp map.
*/
func (interval *Interval) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		interval.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"interval: input is nil",
			nil,
		))

		return
	}

	interval.next.Write(input)
	interval.err = nil
}

/*
Read computes difference between current and previous timestamps.
*/
func (interval *Interval) Read() types.IO[types.Map[string, types.Value[float64]]] {
	in := interval.next.Read()

	if in.Error() != "" {
		interval.err = errnie.Error(errnie.Err(
			errnie.NotFound,
			in.Error(),
			nil,
		))

		return interval.next
	}

	mapping := in.Project().Read()
	tsVal, hasTs := mapping.Get("timestamp")

	if !hasTs {
		interval.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"interval: missing timestamp",
			nil,
		))

		return interval.next
	}

	currentTimestamp := tsVal.Read()
	prevVal, _ := mapping.Get("previous")
	seenVal, hasSeen := mapping.Get("has_seen")

	if !hasSeen || seenVal.Read() == 0 {
		mapping.Put("previous", types.NewValue(currentTimestamp))
		mapping.Put("has_seen", types.NewValue(1.0))
		mapping.Put("delta", types.NewValue(0.0))

		interval.next.Write(types.NewInput(types.NewValue(mapping)))
		interval.err = nil

		return interval.next
	}

	delta := currentTimestamp - prevVal.Read()
	mapping.Put("previous", types.NewValue(currentTimestamp))
	mapping.Put("delta", types.NewValue(delta))

	interval.next.Write(types.NewInput(types.NewValue(mapping)))
	interval.err = nil

	return interval.next
}

/*
Project returns the current projected map.
*/
func (interval *Interval) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return interval.next.Project()
}

/*
Error reports any execution error.
*/
func (interval *Interval) Error() string {
	if interval.err != nil {
		return interval.err.Error()
	}

	return interval.next.Error()
}

/*
Close releases internal state.
*/
func (interval *Interval) Close() error {
	if err := interval.initial.Close(); err != nil {
		return err
	}

	if err := interval.next.Close(); err != nil {
		return err
	}

	interval.err = nil

	return nil
}
