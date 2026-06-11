package market

import (
	"container/ring"

	"github.com/theapemachine/symm/logic"
)

/*
symbolState accumulates one measurement per signal source before a story tick.
*/
type symbolState struct {
	slots  [logic.SourceCount]logic.Measurement
	filled uint16
	ring   *ring.Ring
	walk   *logic.WalkState
}

func newSymbolState(bufferSize int) *symbolState {
	return &symbolState{
		ring: ring.New(bufferSize),
	}
}

func (symbolState *symbolState) absorb(
	measurement logic.Measurement,
) (complete bool, err error) {
	sourceIndex, err := logic.SourceIndex(measurement.Source)

	if err != nil {
		return false, err
	}

	symbolState.slots[sourceIndex] = measurement
	symbolState.filled |= uint16(1) << sourceIndex

	return logic.SpectrumFilled(symbolState.filled), nil
}

func (symbolState *symbolState) resetSpectrum() {
	symbolState.filled = 0
}

func (symbolState *symbolState) appendSpectrum() {
	for _, source := range logic.SpectrumSources {
		sourceIndex, err := logic.SourceIndex(source)

		if err != nil {
			continue
		}

		symbolState.ring.Value = symbolState.slots[sourceIndex]
		symbolState.ring = symbolState.ring.Next()
	}
}

func (symbolState *symbolState) orderedMeasurements() []logic.Measurement {
	ordered := make([]logic.Measurement, 0, symbolState.ring.Len())

	symbolState.ring.Move(1).Do(func(item any) {
		if item == nil {
			return
		}

		ordered = append(ordered, item.(logic.Measurement))
	})

	return ordered
}
