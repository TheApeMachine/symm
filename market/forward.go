package market

import (
	"sync"
	"time"

	"github.com/spf13/viper"
	"github.com/theapemachine/symm/logic"
)

const forwardPendingCap = 64

type forwardPending struct {
	symbol      string
	source      logic.SourceType
	anchorPrice float64
	forecastBps float64
	openedAt    time.Time
}

type forwardPendingQueue struct {
	mu    sync.Mutex
	items []forwardPending
}

type forwardCalibrator struct {
	mu         sync.Mutex
	mse        float64
	scale      float64
	bias       float64
	samples    int
	meanReturn float64
	m2Return   float64
	slopeSeen  bool
}

/*
SettleForwardFeedback matures pending labels against mark prices and refreshes
stored measurement calibration.
*/
func (story *Story) SettleForwardFeedback(
	now time.Time,
	mark func(string) (float64, bool),
) {
	if story == nil || mark == nil || now.IsZero() {
		return
	}

	window := story.forwardWindow()
	alpha := story.forwardSlopeAlpha()

	story.forwardPending.Range(func(_, value any) bool {
		queue := value.(*forwardPendingQueue)

		for _, pending := range queue.settle(now, window) {
			markPrice, ok := mark(pending.symbol)

			if !ok || markPrice <= 0 {
				queue.requeue(pending)

				continue
			}

			realizedBps := forwardReturnBps(pending.anchorPrice, markPrice)
			story.calibratorFor(pending.symbol, pending.source).observe(
				pending.forecastBps,
				realizedBps,
				alpha,
			)
		}

		return true
	})
}

func (story *Story) calibratedSymbolMeasurements(sources *sync.Map) []logic.Measurement {
	measurements := make([]logic.Measurement, 0, logic.SourceCount)

	sources.Range(func(_, measurement any) bool {
		measurements = append(
			measurements,
			story.applyForwardCalibration(measurement.(logic.Measurement)),
		)

		return true
	})

	return measurements
}

/*
FeedbackFor exposes per-source calibration stats for dashboards and tests.
*/
func (story *Story) FeedbackFor(symbol string, source logic.SourceType) *Feedback {
	if story == nil || symbol == "" || source == "" {
		return nil
	}

	raw, ok := story.forwardCal.Load(forwardSourceKey(symbol, source))

	if !ok {
		return nil
	}

	feedback := raw.(*forwardCalibrator).snapshot(symbol)

	if feedback.Samples == 0 {
		return nil
	}

	return feedback
}

func (story *Story) enqueueForwardPending(measurement logic.Measurement) {
	if measurement.Symbol == "" || measurement.Source == "" {
		return
	}

	anchor := measurement.Price

	if anchor <= 0 || !logic.ScalarFinite(anchor) {
		return
	}

	forecastBps := forecastBpsFromMeasurement(measurement)

	if forecastBps == 0 {
		return
	}

	observedAt := measurement.ObservedAt

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	key := forwardSourceKey(measurement.Symbol, measurement.Source)
	raw, _ := story.forwardPending.LoadOrStore(key, &forwardPendingQueue{})
	queue := raw.(*forwardPendingQueue)
	queue.push(forwardPending{
		symbol:      measurement.Symbol,
		source:      measurement.Source,
		anchorPrice: anchor,
		forecastBps: forecastBps,
		openedAt:    observedAt,
	})
}

func (story *Story) applyForwardCalibration(measurement logic.Measurement) logic.Measurement {
	rawBps := forecastBpsFromMeasurement(measurement)

	if rawBps == 0 {
		return measurement
	}

	calibrator := story.calibratorFor(measurement.Symbol, measurement.Source)
	calibratedBps, strengthScale, confidenceScale := calibrator.calibrate(
		rawBps,
		measurement.Confidence,
		story.forwardMinSamples(),
		story.forwardSignificanceZ(),
	)

	measurement.ExpectedMoveBps = calibratedBps

	if strengthScale != 1 {
		measurement.Strength *= strengthScale
	}

	if confidenceScale != 1 {
		measurement.Confidence *= confidenceScale
	}

	return measurement
}

func (story *Story) calibratorFor(symbol string, source logic.SourceType) *forwardCalibrator {
	key := forwardSourceKey(symbol, source)
	raw, _ := story.forwardCal.LoadOrStore(key, &forwardCalibrator{scale: 1})

	return raw.(*forwardCalibrator)
}

func (story *Story) forwardWindow() time.Duration {
	window := viper.GetDuration("market.story.measurement_max_age")

	if window <= 0 {
		window = 30 * time.Second
	}

	return window
}

func (story *Story) forwardMinSamples() int {
	minSamples := viper.GetInt("market.story.forward_return_min_samples")

	if minSamples <= 0 {
		minSamples = 30
	}

	return minSamples
}

func (story *Story) forwardSignificanceZ() float64 {
	z := viper.GetFloat64("market.story.forward_return_significance_z")

	if z <= 0 {
		z = 1.96
	}

	return z
}

func (story *Story) forwardSlopeAlpha() float64 {
	alpha := viper.GetFloat64("market.story.forward_return_slope_alpha")

	if alpha <= 0 || alpha > 1 {
		alpha = 0.05
	}

	return alpha
}

func forwardSourceKey(symbol string, source logic.SourceType) string {
	return symbol + "\x00" + string(source)
}

func forecastBpsFromMeasurement(measurement logic.Measurement) float64 {
	if measurement.ExpectedMoveBps != 0 {
		return measurement.ExpectedMoveBps
	}

	if measurement.Strength <= 0 || measurement.Confidence <= 0 {
		return 0
	}

	sign := 1.0

	if measurement.Position == logic.PositionTypeShort {
		sign = -1
	}

	return sign * measurement.Strength * measurement.Confidence * 100
}

func forwardReturnBps(anchorPrice, markPrice float64) float64 {
	if anchorPrice <= 0 || markPrice <= 0 {
		return 0
	}

	return (markPrice - anchorPrice) / anchorPrice * 10000
}

func (queue *forwardPendingQueue) requeue(pending forwardPending) {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	queue.items = append(queue.items, pending)

	if len(queue.items) > forwardPendingCap {
		queue.items = queue.items[len(queue.items)-forwardPendingCap:]
	}
}

func (queue *forwardPendingQueue) push(pending forwardPending) {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	queue.items = append(queue.items, pending)

	if len(queue.items) > forwardPendingCap {
		queue.items = queue.items[len(queue.items)-forwardPendingCap:]
	}
}

func (queue *forwardPendingQueue) settle(now time.Time, window time.Duration) []forwardPending {
	queue.mu.Lock()
	defer queue.mu.Unlock()

	if len(queue.items) == 0 {
		return nil
	}

	settled := make([]forwardPending, 0, len(queue.items))
	kept := queue.items[:0]

	for _, pending := range queue.items {
		if now.Sub(pending.openedAt) < window {
			kept = append(kept, pending)

			continue
		}

		settled = append(settled, pending)
	}

	queue.items = kept

	return settled
}
