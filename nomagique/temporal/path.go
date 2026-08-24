package temporal

import (
	"fmt"
	"math"
	"sync"

	"github.com/theapemachine/symm/nomagique/types"
	nmtypes "github.com/theapemachine/symm/nomagique/types"
)

const MaxPathSamples = nmtypes.MaxSamples / pathSampleWidth

const pathSampleWidth = 2

/*
Series names one independent path coordinate system inside a Frame. Every slot
the path owns — its ring samples and its control facts — is namespaced by the
series prefix, so one Frame can carry two (or more) independent paths without
collision. The empty prefix reuses the legacy generic sample slots.
*/
type Series struct {
	prefix         string
	ValueSymbol    types.Symbol
	SecSymbol      types.Symbol
	NsecSymbol     types.Symbol
	CountSymbol    types.Symbol
	HeadSymbol     types.Symbol
	ReadySymbol    types.Symbol
	CapacitySymbol types.Symbol
	SpanSymbol     types.Symbol
}

/*
NewSeries resolves the slot table for one series prefix. Control symbols are
interned once; sample symbols are interned lazily on first use so a series only
reserves the sample slots its ring actually touches. The empty prefix resolves
to the legacy generic slots. Construct a Series during pipeline wiring, never
per event.
*/
func NewSeries(prefix string) Series {
	return Series{
		prefix:         prefix,
		ValueSymbol:    types.MustIntern(joinPrefix(prefix, "sample")),
		SecSymbol:      types.MustIntern(joinPrefix(prefix, "unix_sec")),
		NsecSymbol:     types.MustIntern(joinPrefix(prefix, "unix_nsec")),
		CountSymbol:    types.MustIntern(joinPrefix(prefix, "count")),
		HeadSymbol:     types.MustIntern(joinPrefix(prefix, "head")),
		ReadySymbol:    types.MustIntern(joinPrefix(prefix, "ready")),
		CapacitySymbol: types.MustIntern(joinPrefix(prefix, "capacity")),
		SpanSymbol:     types.MustIntern(joinPrefix(prefix, "input/span")),
	}
}

/*
SampleSymbol returns the interned slot for one sample position of the series.
The default series reuses the engine's static generic sample table; prefixed
series intern the position lazily and cache it globally by name.
*/
func (series Series) SampleSymbol(index int) types.Symbol {
	if index < 0 || index >= types.MaxSamples {
		return 0
	}

	if series.prefix == "" {
		return types.MustSampleSymbol(index)
	}

	name := joinPrefix(series.prefix, fmt.Sprintf("sample/%d", index))

	if cached, found := sampleSlotCache.Load(name); found {
		return cached.(types.Symbol)
	}

	symbol := types.MustIntern(name)
	actual, _ := sampleSlotCache.LoadOrStore(name, symbol)

	return actual.(types.Symbol)
}

var sampleSlotCache sync.Map

func joinPrefix(prefix string, name string) string {
	if prefix == "" {
		return name
	}

	return prefix + "/" + name
}

/*
JoinPrefix namespaces one slot name under a series prefix. The empty prefix
returns the name unchanged so the legacy generic slots keep working.
*/
func JoinPrefix(prefix string, name string) string {
	return joinPrefix(prefix, name)
}

/*
DefaultSeries is the unprefixed path coordinate system. It aliases the legacy
generic sample slots, so single-series pipelines remain unchanged.
*/
var DefaultSeries = NewSeries("")

/*
Path returns the retaining ring primitive for one series prefix. Each
observation occupies two sample slots of that series; the timestamp is
bit-encoded so Unix nanoseconds survive Frame storage without float precision
loss.
*/
func Path(prefix string) types.Primitive {
	series := NewSeries(prefix)

	return func(input types.Frame) types.Frame {
		value, hasValue := input.Get(series.ValueSymbol)
		seconds, hasSeconds := input.Get(series.SecSymbol)
		nanoseconds, hasNanoseconds := input.Get(series.NsecSymbol)

		if !hasValue || !hasSeconds || !hasNanoseconds {
			input.Err = fmt.Errorf(
				"temporal: path requires a value and event time",
			)

			return input
		}

		if seconds != math.Trunc(seconds) || nanoseconds != math.Trunc(nanoseconds) ||
			nanoseconds < 0 || nanoseconds >= float64(nanosecondsPerSecond) {
			input.Err = fmt.Errorf(
				"temporal: path requires integral seconds and normalized nanoseconds",
			)

			return input
		}

		timestamp := int64(seconds)*nanosecondsPerSecond + int64(nanoseconds)
		count := series.Count(input)
		head := series.Head(input)
		capacity, err := pathCapacity(series, input, count)

		if err != nil {
			input.Err = err

			return input
		}

		if capacity < count {
			input.Err = fmt.Errorf(
				"temporal: path capacity cannot be smaller than its retained count",
			)

			return input
		}

		if count > 0 {
			latestTimestamp, _, found := series.Sample(&input, count-1)

			if !found || timestamp < latestTimestamp {
				input.Err = fmt.Errorf(
					"temporal: path event time must not regress",
				)

				return input
			}
		}

		physicalIndex := count

		if count >= capacity {
			physicalIndex = head
			head = (head + 1) % capacity
		} else {
			count++
		}

		input.Put(series.SampleSymbol(physicalIndex*pathSampleWidth), math.Float64frombits(uint64(timestamp)))
		input.Put(series.SampleSymbol(physicalIndex*pathSampleWidth+1), value)
		input.Put(series.CountSymbol, float64(count))
		input.Put(series.HeadSymbol, float64(head))
		input.Put(series.ReadySymbol, 1)
		input.Put(series.CapacitySymbol, float64(capacity))

		return input
	}
}

/*
PathSample returns one retained observation of the default series in
chronological order.
*/
func PathSample(frame *types.Frame, index int) (int64, float64, bool) {
	return DefaultSeries.Sample(frame, index)
}

/*
CopyFrom relocates the default-series path retained in src onto dst under this
series' prefix. It is the declarative plumbing for placing two committed paths
into one frame under distinct series prefixes before a bivariate primitive runs.
*/
func (series Series) CopyFrom(dst *types.Frame, src types.Frame) {
	if dst == nil {
		return
	}

	count := DefaultSeries.Count(src)
	capacity, found := src.Get(DefaultSeries.CapacitySymbol)

	if !found {
		return
	}

	for index := 0; index < count; index++ {
		physicalIndex := (DefaultSeries.Head(src) + index) % int(capacity)
		timestampBits, hasTimestamp := src.Get(DefaultSeries.SampleSymbol(physicalIndex*pathSampleWidth))
		value, hasValue := src.Get(DefaultSeries.SampleSymbol(physicalIndex*pathSampleWidth+1))

		if !hasTimestamp || !hasValue {
			continue
		}

		dst.Put(series.SampleSymbol(index*pathSampleWidth), timestampBits)
		dst.Put(series.SampleSymbol(index*pathSampleWidth+1), value)
	}

	dst.Put(series.CountSymbol, float64(count))
	dst.Put(series.HeadSymbol, 0)
	dst.Put(series.CapacitySymbol, capacity)

	if ready, ok := src.Get(DefaultSeries.ReadySymbol); ok {
		dst.Put(series.ReadySymbol, ready)
	}
}

/*
Count reports how many observations the series retains in frame.
*/
func (series Series) Count(frame types.Frame) int {
	count, found := frame.Get(series.CountSymbol)

	if !found {
		return 0
	}

	return int(count)
}

/*
Head reports the physical ring head of the series in frame.
*/
func (series Series) Head(frame types.Frame) int {
	head, found := frame.Get(series.HeadSymbol)

	if !found {
		return 0
	}

	return int(head)
}

/*
Sample returns one retained observation of the series in chronological order.
*/
func (series Series) Sample(frame *types.Frame, index int) (int64, float64, bool) {
	if frame == nil {
		return 0, 0, false
	}

	count := series.Count(*frame)
	capacity, found := frame.Get(series.CapacitySymbol)

	if !found || index < 0 || index >= count {
		return 0, 0, false
	}

	physicalIndex := (series.Head(*frame) + index) % int(capacity)
	timestampBits, hasTimestamp := frame.Get(
		series.SampleSymbol(physicalIndex*pathSampleWidth),
	)
	value, hasValue := frame.Get(
		series.SampleSymbol(physicalIndex*pathSampleWidth+1),
	)

	if !hasTimestamp || !hasValue {
		return 0, 0, false
	}

	timestamp := int64(math.Float64bits(timestampBits))

	return timestamp, value, true
}

/*
SampleAt returns the value stored at one physical slot of the series. Window
style rings use the series sample slots one per observation; path style rings
use them two per observation. One series must pick one style.
*/
func (series Series) SampleAt(frame *types.Frame, physicalIndex int) (float64, bool) {
	if frame == nil || physicalIndex < 0 || physicalIndex >= types.MaxSamples {
		return 0, false
	}

	return frame.Get(series.SampleSymbol(physicalIndex))
}

func pathCapacity(series Series, input types.Frame, count int) (int, error) {
	capacity, found := input.Get(series.SpanSymbol)

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

	capacityValue, _ := input.Get(series.CapacitySymbol)

	if count < int(capacityValue) {
		return int(capacityValue), nil
	}

	capacity = capacityValue + capacityValue

	if capacity > MaxPathSamples {
		capacity = MaxPathSamples
	}

	return int(capacity), nil
}

const nanosecondsPerSecond = int64(1_000_000_000)
