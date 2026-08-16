package temporal

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

type durationMap = types.Map[string, types.Value[float64]]

/*
Duration computes an exact-enough elapsed second value from separate second and
nanosecond coordinates. Its map carries current_sec/current_nsec,
previous_sec/previous_nsec, and delta.
*/
type Duration struct {
	initial types.Input[durationMap]
	next    types.Input[durationMap]
}

var _ types.IO[durationMap] = (*Duration)(nil)

func NewDuration(initial types.Input[durationMap]) *Duration {
	return &Duration{initial: initial, next: types.NewInput[durationMap]()}
}

func (duration *Duration) Write(input types.IO[durationMap]) {
	if input == nil {
		mapping := types.NewMap[string, types.Value[float64]]()
		duration.next = types.NewErrorInput(mapping, durationError("input is nil"))
		return
	}
	if input.Error() != "" {
		mapping := types.NewMap[string, types.Value[float64]]()
		duration.next = types.NewErrorInput(mapping,
			errnie.Error(errnie.Err(errnie.NotFound, input.Error(), nil)))
		return
	}
	duration.next = types.NewInput(types.NewValue(input.Project().Read()))
}

func (duration *Duration) Read() types.IO[durationMap] {
	if duration.next.Error() != "" {
		return duration.next
	}

	mapping := duration.next.Project().Read()
	currentSec, okCurrentSec := durationNumber(mapping, "current_sec")
	currentNsec, okCurrentNsec := durationNumber(mapping, "current_nsec")
	previousSec, okPreviousSec := durationNumber(mapping, "previous_sec")
	previousNsec, okPreviousNsec := durationNumber(mapping, "previous_nsec")
	if !okCurrentSec || !okCurrentNsec || !okPreviousSec || !okPreviousNsec {
		duration.next = types.NewErrorInput(mapping,
			durationError("missing current or previous timestamp coordinate"))
		return duration.next
	}
	if !durationFinite(currentSec, currentNsec, previousSec, previousNsec) ||
		currentNsec < 0 || currentNsec >= 1e9 || previousNsec < 0 || previousNsec >= 1e9 {
		duration.next = types.NewErrorInput(mapping,
			durationError("timestamp coordinates must be finite and normalized"))
		return duration.next
	}

	delta := (currentSec - previousSec) + (currentNsec-previousNsec)*1e-9
	mapping.Put("delta", types.NewValue(delta))
	duration.initial = types.NewInput(types.NewValue(mapping))
	duration.next = types.NewInput(types.NewValue(mapping))
	return duration.next
}

func (duration *Duration) Project() types.Value[durationMap] { return duration.next.Project() }
func (duration *Duration) Error() string                     { return duration.next.Error() }
func (duration *Duration) Close() error {
	if duration.initial != nil {
		if err := duration.initial.Close(); err != nil {
			return err
		}
	}
	if duration.next != nil {
		if err := duration.next.Close(); err != nil {
			return err
		}
	}
	duration.next = types.NewInput[durationMap]()
	return nil
}

func durationNumber(mapping durationMap, key string) (float64, bool) {
	value, found := mapping.Get(key)
	if !found {
		return 0, false
	}
	return value.Read(), true
}

func durationFinite(values ...float64) bool {
	for _, value := range values {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
	}
	return true
}

func durationError(message string) error {
	return errnie.Error(errnie.Err(errnie.Validation, "duration: "+message, nil))
}
