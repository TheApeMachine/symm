package telemetry

import (
	"container/ring"
	"errors"
	"fmt"
	"math"
	"time"

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
	bus              *internal.Bus
	source           string
	readings         *ring.Ring
	warmupCapacity   int
	warmupSamples    int
	minWarmupSamples int
	expectedSymbols  map[string]struct{}
	lastPublishAt    time.Time
}

/*
NewGauge binds one signal source to the shared ui broadcast.
*/
func NewGauge(bus *internal.Bus, source logic.SourceType) (*Gauge, error) {
	signalName := string(source)

	if source == logic.SourceExhaustion {
		signalName = "exhaust"
	}

	warmupCapacity := viper.GetInt(fmt.Sprintf("signals.%s.measurements_capacity", signalName))

	if warmupCapacity <= 0 {
		return nil, errors.New("telemetry gauge: measurements_capacity must be positive")
	}

	return &Gauge{
		bus:             bus,
		source:          string(source),
		readings:        ring.New(viper.GetInt("telemetry.gauge.readings_capacity")),
		warmupCapacity:  warmupCapacity,
		expectedSymbols: make(map[string]struct{}),
	}, nil
}

/*
RegisterSymbols is retained for callers that batch-announce the universe.
Warmup denominators are fixed lazily when a symbol first contributes a
warmed Record so inactive pairs do not stall the gauge.
*/
func (gauge *Gauge) RegisterSymbols(symbols []string) {
	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		gauge.expectedSymbols[symbol] = struct{}{}
	}
}

/*
RecordWarmup counts a warmed signal write toward the dashboard gauge bar.
It must run for every Record, even when Measure fails afterward.
*/
func (gauge *Gauge) RecordWarmup(symbol string, warmed bool) {
	if symbol == "" || !warmed {
		return
	}

	if _, registered := gauge.expectedSymbols[symbol]; !registered {
		gauge.expectedSymbols[symbol] = struct{}{}
		gauge.minWarmupSamples += gauge.warmupCapacity
	}

	gauge.warmupSamples++
}

/*
PublishWarmup rebroadcasts warmup progress to the dashboard gauge bars.
*/
func (gauge *Gauge) PublishWarmup() error {
	if gauge.publishThrottled() {
		return nil
	}

	samples, minSamples, calibrating, calibrated := gauge.warmupState()

	gauge.lastPublishAt = time.Now()

	return gauge.bus.Send(internal.ChannelUI, "gauge", map[string]any{
		"chart":       "gauge",
		"source":      gauge.source,
		"confidence":  0.0,
		"surprise":    0.0,
		"samples":     samples,
		"min_samples": minSamples,
		"calibrating": calibrating,
		"calibrated":  calibrated,
	})
}

/*
Publish records the symbol reading and rebroadcasts mean confidence and SNR.
*/
func (gauge *Gauge) Publish(
	measurement logic.Measurement,
	symbol string,
) error {
	gauge.readings.Value = &Reading{
		Confidence: measurement.Confidence,
		Surprise:   measurement.Surprise,
	}

	gauge.readings = gauge.readings.Next()

	if gauge.publishThrottled() {
		return nil
	}

	meanConfidence, meanSurprise := gauge.readingMeans()

	samples, minSamples, calibrating, calibrated := gauge.warmupState()

	gauge.lastPublishAt = time.Now()

	threshold := gauge.surpriseThreshold()

	RecordSurpriseRatio(gauge.source, meanSurprise, threshold)

	return gauge.bus.Send(internal.ChannelUI, "gauge", map[string]any{
		"chart":              "gauge",
		"source":             gauge.source,
		"confidence":         meanConfidence,
		"surprise":           meanSurprise,
		"surprise_threshold": threshold,
		"samples":            samples,
		"min_samples":        minSamples,
		"calibrating":        calibrating,
		"calibrated":         calibrated,
	})
}

func (gauge *Gauge) readingMeans() (meanConfidence float64, meanSurprise float64) {
	confidenceCount := 0.0
	surpriseCount := 0.0

	gauge.readings.Do(func(value any) {
		if value == nil {
			return
		}

		reading, ok := value.(*Reading)

		if !ok {
			return
		}

		if reading.Confidence > 0 {
			meanConfidence += reading.Confidence
			confidenceCount++
		}

		if reading.Surprise > 0 {
			meanSurprise += reading.Surprise
			surpriseCount++
		}
	})

	if confidenceCount > 0 {
		meanConfidence /= confidenceCount
	}

	if surpriseCount > 0 {
		meanSurprise /= surpriseCount
	}

	return meanConfidence, meanSurprise
}

func (gauge *Gauge) warmupState() (
	samples int,
	minSamples int,
	calibrating bool,
	calibrated bool,
) {
	minSamples = gauge.warmupCapacity

	if len(gauge.expectedSymbols) == 0 {
		return 0, minSamples, true, false
	}

	samples = gauge.warmupSamples
	minSamples = gauge.minWarmupSamples
	calibrated = gauge.warmupSamples >= gauge.minWarmupSamples
	calibrating = !calibrated

	return samples, minSamples, calibrating, calibrated
}

func (gauge *Gauge) publishThrottled() bool {
	interval := viper.GetDuration("telemetry.gauge.publish_interval")

	if interval <= 0 {
		interval = 100 * time.Millisecond
	}

	if gauge.lastPublishAt.IsZero() {
		return false
	}

	return time.Since(gauge.lastPublishAt) < interval
}

func (gauge *Gauge) surpriseThreshold() float64 {
	thresholdKey := fmt.Sprintf("signals.%s.surprise_threshold", gauge.source)

	if gauge.source == string(logic.SourceSentiment) {
		thresholdKey = "signals.sentiment.surge_threshold"
	}

	if gauge.source == string(logic.SourceExhaustion) {
		thresholdKey = "signals.exhaust.surprise_threshold"
	}

	return math.Min(math.Max(viper.GetFloat64(thresholdKey), 1.0), 5.0)
}
