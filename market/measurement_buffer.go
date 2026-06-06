package market

import (
	"errors"
	"fmt"

	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

const storyMeasurementsBufferKey = "story.measurements.buffer"

/*
MeasurementBuffer returns the configured story measurement ring buffer size.
*/
func MeasurementBuffer() (int, error) {
	if !viper.IsSet(storyMeasurementsBufferKey) {
		return 0, errnie.Error(errors.New("story: story.measurements.buffer not configured"))
	}

	configured := viper.GetInt(storyMeasurementsBufferKey)

	if configured <= 0 {
		return 0, errnie.Error(fmt.Errorf(
			"story: story.measurements.buffer must be positive, got %d",
			configured,
		))
	}

	return configured, nil
}
