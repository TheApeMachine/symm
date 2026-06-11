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
	bus                    *internal.Bus
	source                 string
	readings               *ring.Ring
	warmupCapacity         int
	warmupSamples          int
	minWarmupSamples       int
	expectedSymbols        map[string]struct{}
	warmupSymbols          map[string]struct{}
	warmupHandoffPublished bool
	lastPublishAt          time.Time
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
		warmupSymbols:   make(map[string]struct{}),
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

	if _, counted := gauge.warmupSymbols[symbol]; !counted {
		gauge.warmupSymbols[symbol] = struct{}{}
		gauge.minWarmupSamples += gauge.warmupCapacity
	}

	gauge.warmupSamples++
}

/*
PublishWarmup rebroadcasts warmup progress to the dashboard gauge bars.
*/
func (gauge *Gauge) PublishWarmup() error {
	samples, minSamples, calibrating, calibrated := gauge.warmupState()

	if calibrated {
		if gauge.warmupHandoffPublished {
			return nil
		}

		gauge.warmupHandoffPublished = true
		gauge.lastPublishAt = time.Now()

		return gauge.publishFrame(0, 0, samples, minSamples, false, true)
	}

	if !calibrating {
		return nil
	}

	if gauge.publishThrottled() {
		return nil
	}

	gauge.lastPublishAt = time.Now()

	return gauge.publishFrame(0, 0, samples, minSamples, calibrating, calibrated)
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

	if gauge.publishThrottled() {
		return nil
	}

	gauge.lastPublishAt = time.Now()

	return gauge.publishReadingFrame()
}

/*
PublishNow rebroadcasts mean confidence and SNR without throttling.
*/
func (gauge *Gauge) PublishNow() error {
	gauge.lastPublishAt = time.Now()

	return gauge.publishReadingFrame()
}

func (gauge *Gauge) publishReadingFrame() error {
	meanConfidence, meanSurprise := gauge.readingMeans()

	threshold := gauge.surpriseThreshold()

	RecordSurpriseRatio(gauge.source, meanSurprise, threshold)

	// Measurements publish from tick one with honest 1/N confidence; warmup never
	// replaces confidence on the dashboard needle.
	return gauge.publishFrame(
		meanConfidence,
		meanSurprise,
		0,
		0,
		false,
		true,
	)
}

func (gauge *Gauge) publishFrame(
	meanConfidence float64,
	meanSurprise float64,
	samples int,
	minSamples int,
	calibrating bool,
	calibrated bool,
) error {
	threshold := gauge.surpriseThreshold()

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

	if gauge.minWarmupSamples == 0 {
		return samples, minSamples, true, false
	}

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
