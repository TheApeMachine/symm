package telemetry

import (
	"container/ring"
	"fmt"
	"math"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/internal"
	"github.com/theapemachine/symm/logic"
)

type Reading struct {
	Confidence float64
	Surprise   float64
	Strength   float64
	Elapsed    float64
	ObservedAt time.Time
	Active     bool
}

type gaugeFrame struct {
	meanConfidence   float64
	meanSurprise     float64
	meanStrength     float64
	meanElapsed      float64
	samples          int
	minSamples       int
	activeReadings   int
	readingsCapacity int
	calibrating      bool
	calibrated       bool
	observedAt       time.Time
}

type gaugeSummary struct {
	meanConfidence   float64
	meanSurprise     float64
	meanStrength     float64
	meanElapsed      float64
	activeReadings   int
	readingsCapacity int
	latestObservedAt time.Time
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
	publishInterval        time.Duration
	surpriseThresholdValue float64
}

/*
NewGauge binds one signal source to the shared ui broadcast.
*/
func NewGauge(bus *internal.Bus, source logic.SourceType) (*Gauge, error) {
	warmupCapacity, capacityErr := config.BaseMeasurementCapacity()

	if capacityErr != nil {
		return nil, capacityErr
	}

	publishInterval := viper.GetDuration("telemetry.gauge.publish_interval")

	if publishInterval <= 0 {
		publishInterval = 100 * time.Millisecond
	}

	return &Gauge{
		bus:                    bus,
		source:                 string(source),
		readings:               ring.New(viper.GetInt("telemetry.gauge.readings_capacity")),
		warmupCapacity:         warmupCapacity,
		expectedSymbols:        make(map[string]struct{}),
		warmupSymbols:          make(map[string]struct{}),
		publishInterval:        publishInterval,
		surpriseThresholdValue: gaugeSurpriseThreshold(source),
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

		return gauge.publishFrame(gaugeFrame{
			samples:    samples,
			minSamples: minSamples,
			calibrated: true,
		})
	}

	if !calibrating {
		return nil
	}

	if gauge.publishThrottled() {
		return nil
	}

	gauge.lastPublishAt = time.Now()

	return gauge.publishFrame(gaugeFrame{
		samples:     samples,
		minSamples:  minSamples,
		calibrating: calibrating,
		calibrated:  calibrated,
	})
}

/*
Publish records the symbol reading and rebroadcasts mean confidence and SNR.
*/
func (gauge *Gauge) Publish(measurement logic.Measurement) error {
	gauge.readings.Value = &Reading{
		Confidence: measurement.Confidence,
		Surprise:   measurement.Surprise,
		Strength:   measurement.Strength,
		Elapsed:    measurement.Elapsed,
		ObservedAt: measurement.ObservedAt,
		Active: measurement.Publishable() &&
			!measurement.BestEffort &&
			measurement.GapReason == "",
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
	summary := gauge.readingSummary()

	threshold := gauge.surpriseThreshold()

	RecordSurpriseRatio(gauge.source, summary.meanSurprise, threshold)

	// Measurements publish from tick one with honest 1/N confidence; warmup never
	// replaces confidence on the dashboard needle.
	return gauge.publishFrame(gaugeFrame{
		meanConfidence:   summary.meanConfidence,
		meanSurprise:     summary.meanSurprise,
		meanStrength:     summary.meanStrength,
		meanElapsed:      summary.meanElapsed,
		activeReadings:   summary.activeReadings,
		readingsCapacity: summary.readingsCapacity,
		calibrated:       true,
		observedAt:       summary.latestObservedAt,
	})
}

func (gauge *Gauge) publishFrame(frame gaugeFrame) error {
	threshold := gauge.surpriseThreshold()

	payload := map[string]any{
		"chart":              "gauge",
		"source":             gauge.source,
		"confidence":         frame.meanConfidence,
		"surprise":           frame.meanSurprise,
		"surprise_threshold": threshold,
		"strength":           frame.meanStrength,
		"elapsed":            frame.meanElapsed,
		"active_readings":    frame.activeReadings,
		"readings_capacity":  frame.readingsCapacity,
		"samples":            frame.samples,
		"min_samples":        frame.minSamples,
		"calibrating":        frame.calibrating,
		"calibrated":         frame.calibrated,
	}

	if !frame.observedAt.IsZero() {
		payload["observed_at"] = frame.observedAt.Format(time.RFC3339Nano)
	}

	return gauge.bus.Send(internal.ChannelUI, "gauge", payload)
}

func (gauge *Gauge) readingMeans() (meanConfidence float64, meanSurprise float64) {
	summary := gauge.readingSummary()

	return summary.meanConfidence, summary.meanSurprise
}

func (gauge *Gauge) readingSummary() gaugeSummary {
	summary := gaugeSummary{}

	if gauge.readings == nil {
		return summary
	}

	summary.readingsCapacity = gauge.readings.Len()

	confidenceCount := 0.0
	surpriseCount := 0.0
	strengthCount := 0.0
	elapsedCount := 0.0

	gauge.readings.Do(func(value any) {
		if value == nil {
			return
		}

		reading, ok := value.(*Reading)

		if !ok {
			return
		}

		if reading.Confidence > 0 {
			summary.meanConfidence += reading.Confidence
			confidenceCount++
		}

		if reading.Surprise > 0 {
			summary.meanSurprise += reading.Surprise
			surpriseCount++
		}

		if reading.Strength > 0 {
			summary.meanStrength += reading.Strength
			strengthCount++
		}

		if reading.Elapsed > 0 {
			summary.meanElapsed += reading.Elapsed
			elapsedCount++
		}

		if reading.Active {
			summary.activeReadings++
		}

		if reading.ObservedAt.After(summary.latestObservedAt) {
			summary.latestObservedAt = reading.ObservedAt
		}
	})

	if confidenceCount > 0 {
		summary.meanConfidence /= confidenceCount
	}

	if surpriseCount > 0 {
		summary.meanSurprise /= surpriseCount
	}

	if strengthCount > 0 {
		summary.meanStrength /= strengthCount
	}

	if elapsedCount > 0 {
		summary.meanElapsed /= elapsedCount
	}

	return summary
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
	if gauge.lastPublishAt.IsZero() {
		return false
	}

	return time.Since(gauge.lastPublishAt) < gauge.publishInterval
}

func (gauge *Gauge) surpriseThreshold() float64 {
	return gauge.surpriseThresholdValue
}

func gaugeSurpriseThreshold(source logic.SourceType) float64 {
	thresholdKey := fmt.Sprintf("signals.%s.surprise_threshold", source)

	if source == logic.SourceSentiment {
		thresholdKey = "signals.sentiment.surge_threshold"
	}

	if source == logic.SourceExhaustion {
		thresholdKey = "signals.exhaust.surprise_threshold"
	}

	return math.Min(math.Max(viper.GetFloat64(thresholdKey), 1.0), 5.0)
}
