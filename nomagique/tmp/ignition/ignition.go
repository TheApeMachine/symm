package equation

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

type Ignition struct {
	initial types.Input[types.Map[string, types.Value[float64]]]
	next    types.Input[types.Map[string, types.Value[float64]]]
	err     error
}

func NewIgnition(initial types.Input[types.Map[string, types.Value[float64]]]) *Ignition {
	return &Ignition{
		initial: initial,
		next:    types.NewInput[types.Map[string, types.Value[float64]]](),
	}
}

func (ign *Ignition) Write(input types.IO[types.Map[string, types.Value[float64]]]) {
	if input == nil {
		ign.err = errnie.Error(errnie.Err(
			errnie.Validation,
			"ignition: input is nil",
			nil,
		))
		return
	}
	ign.next.Write(input)
	ign.err = nil
}

func (ign *Ignition) Read() types.IO[types.Map[string, types.Value[float64]]] {
	// Stage 1: Squash RVOL
	squash := NewSquash(ign.initial)
	ign.next.Write(squash)
	if squash.err != nil {
		ign.err = squash.err
		return ign.next
	}

	// Stage 2: Inverse compression
	inverse := NewInverse(ign.next)
	ign.next.Write(inverse)
	if inverse.err != nil {
		ign.err = inverse.err
		return ign.next
	}

	// Stage 3: Ratio normalization
	ratio := NewRatio(ign.next)
	ign.next.Write(ratio)
	if ratio.err != nil {
		ign.err = ratio.err
		return ign.next
	}

	// Stage 4: Mean of evidence
	mean := NewMean(ign.next)
	ign.next.Write(mean)
	if mean.err != nil {
		ign.err = mean.err
		return ign.next
	}

	// Stage 5: Exhaustion calculation
	exhaustion := NewExhaustion(ign.next)
	ign.next.Write(exhaustion)
	if exhaustion.err != nil {
		ign.err = exhaustion.err
		return ign.next
	}

	// Stage 6: Ratio scale
	ratioScale := NewRatioScale(ign.next)
	ign.next.Write(ratioScale)
	if ratioScale.err != nil {
		ign.err = ratioScale.err
		return ign.next
	}

	// Read final result
	result := ign.next.Read()

	if result.Error() != "" {
		ign.err = errnie.Error(errnie.NotFound, result.Error(), nil)
		return ign.next
	}

	// Extract values from the composed map
	mapping := result.Project().Read()
	ignitionResult, hasResult := mapping.Get("result")

	if !hasResult {
		ign.err = errnie.Error(errnie.Validation,
			"ignition: missing result in composed map", nil)
		return ign.next
	}

	resultValue := ignitionResult.Read()

	mapping.Put("result", types.NewValue(resultValue))

	ign.next.Write(types.NewInput(types.NewValue(mapping)))
	ign.err = nil

	return ign.next
}

func (ign *Ignition) Project() types.Value[types.Map[string, types.Value[float64]]] {
	return ign.next.Project()
}

func (ign *Ignition) Error() string {
	if ign.err != nil {
		return ign.err.Error()
	}
	return ign.next.Error()
}

func (ign *Ignition) Close() error {
	if err := ign.initial.Close(); err != nil {
		return err
	}
	if err := ign.next.Close(); err != nil {
		return err
	}
	ign.err = nil
	return nil
}