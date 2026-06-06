package market

import (
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
	story.ringWindow.Value = measurement
	story.ringWindow = story.ringWindow.Next()
	story.ringPtr++
}
