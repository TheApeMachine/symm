package transport

import (
	"fmt"
	"math"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/types"
)

type windowMap = types.Map[string, types.Value[float64]]

/*
Window retains a bounded ring of scalar samples entirely inside its map. The
map carries "capacity", the staged "sample", "count", "head", and
"sample/<slot>" entries.
*/
type Window struct {
	initial types.Input[windowMap]
	next    types.Input[windowMap]
}

var _ types.IO[windowMap] = (*Window)(nil)

func NewWindow(initial types.Input[windowMap]) *Window {
	return &Window{initial: initial, next: types.NewInput[windowMap]()}
}

func (window *Window) Write(input types.IO[windowMap]) {
	if input == nil {
		mapping := types.NewMap[string, types.Value[float64]]()
		window.next = types.NewErrorInput(mapping, windowError("input is nil"))
		return
	}
	if input.Error() != "" {
		mapping := types.NewMap[string, types.Value[float64]]()
		window.next = types.NewErrorInput(mapping,
			errnie.Error(errnie.Err(errnie.NotFound, input.Error(), nil)))
		return
	}
	window.next = types.NewInput(types.NewValue(input.Project().Read()))
}

func (window *Window) Read() types.IO[windowMap] {
	if window.next.Error() != "" {
		return window.next
	}

	mapping := window.next.Project().Read()
	capacityValue, hasCapacity := mapping.Get("capacity")
	sampleValue, hasSample := mapping.Get("sample")
	if !hasCapacity || !hasSample {
		window.next = types.NewErrorInput(mapping,
			windowError("missing capacity or sample"))
		return window.next
	}

	capacityFloat := capacityValue.Read()
	sample := sampleValue.Read()
	if capacityFloat <= 0 || capacityFloat != math.Trunc(capacityFloat) ||
		math.IsNaN(capacityFloat) || math.IsInf(capacityFloat, 0) {
		window.next = types.NewErrorInput(mapping,
			windowError("capacity must be a positive integer"))
		return window.next
	}
	if math.IsNaN(sample) || math.IsInf(sample, 0) {
		window.next = types.NewErrorInput(mapping,
			windowError("sample must be finite"))
		return window.next
	}

	capacity := int(capacityFloat)
	count := windowInteger(mapping, "count")
	head := windowInteger(mapping, "head")
	if count < 0 || count > capacity || head < 0 || head >= capacity {
		window.next = types.NewErrorInput(mapping,
			windowError("invalid retained window state"))
		return window.next
	}

	slot := count
	if count >= capacity {
		slot = head
		head = (head + 1) % capacity
	} else {
		count++
	}

	mapping.Put(fmt.Sprintf("sample/%d", slot), types.NewValue(sample))
	mapping.Put("count", types.NewValue(float64(count)))
	mapping.Put("head", types.NewValue(float64(head)))
	mapping.Put("ready", types.NewValue(1.0))
	window.initial = types.NewInput(types.NewValue(mapping))
	window.next = types.NewInput(types.NewValue(mapping))
	return window.next
}

func (window *Window) Project() types.Value[windowMap] { return window.next.Project() }
func (window *Window) Error() string                   { return window.next.Error() }
func (window *Window) Close() error {
	if window.initial != nil {
		if err := window.initial.Close(); err != nil {
			return err
		}
	}
	if window.next != nil {
		if err := window.next.Close(); err != nil {
			return err
		}
	}
	window.next = types.NewInput[windowMap]()
	return nil
}

/*
Samples returns only retained sample slots, excluding transport metadata.
*/
func Samples(mapping windowMap) windowMap {
	samples := types.NewMap[string, types.Value[float64]]()
	for key, value := range mapping.All() {
		if strings.HasPrefix(key, "sample/") {
			samples.Put(key, value)
		}
	}
	return samples
}

func windowInteger(mapping windowMap, key string) int {
	value, found := mapping.Get(key)
	if !found {
		return 0
	}
	return int(value.Read())
}

func windowError(message string) error {
	return errnie.Error(errnie.Err(errnie.Validation, "window: "+message, nil))
}
