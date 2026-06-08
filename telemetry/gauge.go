package telemetry

import (
	"container/ring"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
)

type Reading struct {
	Confidence float64
	Surprise   float64
}

/*
Gauge tracks the latest per-symbol confidence and SNR for one signal source and
publishes cross-section means to the ui bus for the dashboard gauge wire.
*/
type Gauge struct {
	bus      *internal.Bus
	source   string
	readings *ring.Ring
}

/*
NewGauge binds one signal source to the shared ui broadcast.
*/
func NewGauge(bus *internal.Bus, source logic.SourceType) (*Gauge, error) {
	return &Gauge{
		bus:      bus,
		source:   string(source),
		readings: ring.New(viper.GetInt("telemetry.gauge.readings_capacity")),
	}, nil
}

/*
Publish records the symbol reading and rebroadcasts mean confidence and SNR.
*/
func (gauge *Gauge) Publish(measurement logic.Measurement) error {
	gauge.readings.Value = &Reading{
		Confidence: measurement.Confidence,
		Surprise:   measurement.Surprise,
	}

	gauge.readings = gauge.readings.Next()

	meanConfidence := 0.0
	meanSurprise := 0.0
	confidenceCount := 0.0
	surpriseCount := 0.0

	gauge.readings.Do(func(value any) {
		reading := value.(*Reading)

		if reading.Confidence > 0 {
			meanConfidence += reading.Confidence
			confidenceCount++
		}

		if reading.Surprise > 0 {
			meanSurprise += reading.Surprise
			surpriseCount++
		}
	})

	meanConfidence /= confidenceCount
	meanSurprise /= surpriseCount

	return gauge.bus.Send("ui", "gauge", map[string]any{
		"chart":      "gauge",
		"source":     gauge.source,
		"confidence": meanConfidence,
		"surprise":   meanSurprise,
	})
}
