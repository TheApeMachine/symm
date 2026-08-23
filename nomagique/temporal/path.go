package temporal

import (
	"fmt"
	"math"

	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

const MaxPathSamples = nmtypes.MaxSamples / pathSampleWidth

const pathSampleWidth = 2

/*
Path retains an ordered ring of exact event timestamps and numeric values. Each
observation occupies two generic sample slots; the timestamp is bit-encoded so
Unix nanoseconds survive Frame storage without float precision loss.
*/
func Path(
	state types.Frame,
	input types.Frame,
) (types.Frame, types.Frame, error) {
	value, hasValue := input.Get(nmtypes.SampleValue)
	seconds, hasSeconds := input.Get(SymbolUnixSec)
	nanoseconds, hasNanoseconds := input.Get(SymbolUnixNsec)

	if !hasValue || !hasSeconds || !hasNanoseconds {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: path requires a value and event time",
		)
	}

	if seconds != math.Trunc(seconds) || nanoseconds != math.Trunc(nanoseconds) ||
		nanoseconds < 0 || nanoseconds >= float64(nanosecondsPerSecond) {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: path requires integral seconds and normalized nanoseconds",
		)
	}

	timestamp := int64(seconds)*nanosecondsPerSecond + int64(nanoseconds)
	count := pathCount(state)
	head := pathHead(state)
	capacity, err := pathCapacity(state, input, count)

	if err != nil {
		return state, types.Frame{}, err
	}

	if capacity < count {
		return state, types.Frame{}, fmt.Errorf(
			"temporal: path capacity cannot be smaller than its retained count",
		)
	}

	if count > 0 {
		latestTimestamp, _, found := PathSample(&state, count-1)

		if !found || timestamp < latestTimestamp {
			return state, types.Frame{}, fmt.Errorf(
				"temporal: path event time must not regress",
			)
		}
	}

	nextState := state
	physicalIndex := count

	if count >= capacity {
		physicalIndex = head
		head = (head + 1) % capacity
	} else {
		count++
	}

	putPathSample(&nextState, physicalIndex, timestamp, value)
	nextState.Put(nmtypes.SampleCount, float64(count))
	nextState.Put(nmtypes.SampleHead, float64(head))
	nextState.Put(nmtypes.SampleReady, 1)
	nextState.Put(SymbolCapacity, float64(capacity))

	output := nextState
	output.Merge(input)

	return nextState, output, nil
}

/*
PathSample returns one retained observation in chronological order.
*/
func PathSample(frame *types.Frame, index int) (int64, float64, bool) {
	if frame == nil {
		return 0, 0, false
	}

	count := pathCount(*frame)
	capacity, found := frame.Get(SymbolCapacity)

	if !found || index < 0 || index >= count {
		return 0, 0, false
	}

	physicalIndex := (pathHead(*frame) + index) % int(capacity)
	timestampBits, hasTimestamp := frame.Get(
		nmtypes.MustSampleSymbol(physicalIndex * pathSampleWidth),
	)
	value, hasValue := frame.Get(
		nmtypes.MustSampleSymbol(physicalIndex*pathSampleWidth + 1),
	)

	if !hasTimestamp || !hasValue {
		return 0, 0, false
	}

	timestamp := int64(math.Float64bits(timestampBits))

	return timestamp, value, true
}

func putPathSample(
	frame *types.Frame,
	index int,
	timestamp int64,
	value float64,
) {
	frame.Put(
		nmtypes.MustSampleSymbol(index*pathSampleWidth),
		math.Float64frombits(uint64(timestamp)),
	)
	frame.Put(nmtypes.MustSampleSymbol(index*pathSampleWidth+1), value)
}

func pathCapacity(
	state types.Frame,
	input types.Frame,
	count int,
) (int, error) {
	capacity, found := input.Get(nmtypes.Span)

	if !found {
		capacity, found = state.Get(nmtypes.Span)
	}

	if found {
		if capacity < 1 || capacity > MaxPathSamples || capacity != math.Trunc(capacity) {
			return 0, fmt.Errorf(
				"temporal: path span must be an integer from 1 through %d",
				MaxPathSamples,
			)
		}

		return int(capacity), nil
	}

	if count < 1 {
		return 1, nil
	}

	capacityValue, _ := state.Get(SymbolCapacity)

	if count < int(capacityValue) {
		return int(capacityValue), nil
	}

	capacity = capacityValue + capacityValue

	if capacity > MaxPathSamples {
		capacity = MaxPathSamples
	}

	return int(capacity), nil
}

func pathCount(frame types.Frame) int {
	count, found := frame.Get(nmtypes.SampleCount)

	if !found {
		return 0
	}

	return int(count)
}

func pathHead(frame types.Frame) int {
	head, found := frame.Get(nmtypes.SampleHead)

	if !found {
		return 0
	}

	return int(head)
}

const nanosecondsPerSecond = int64(1_000_000_000)
