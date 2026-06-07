package market

import (
	"container/ring"
	"fmt"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/market/perspectives/types"
)

func (story *Story) recordMeasurement(measurement types.Measurement) {
	if story.recorder == nil {
		return
	}

	if measurement.At.IsZero() {
		measurement.At = time.Now().UTC()
	}

	raw, err := sonic.Marshal(measurement)

	if err != nil {
		errnie.Error(fmt.Errorf("marshal measurement: %w", err))
		return
	}

	if _, err = story.recorder.Write(append(raw, '\n')); err != nil {
		errnie.Error(fmt.Errorf("write measurement record: %w", err))
	}
}

func (story *Story) rememberMeasurement(measurement types.Measurement) {
	symbol := measurement.Symbol

	if symbol == "" {
		return
	}

	window, ok := story.symbolWindows[symbol]

	if !ok {
		window = ring.New(story.windowCapacity)
		story.symbolWindows[symbol] = window
	}

	window.Value = measurement
	story.symbolWindows[symbol] = window.Next()
}
