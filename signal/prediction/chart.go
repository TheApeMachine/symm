package prediction

import (
	"math"
	"time"

	"github.com/theapemachine/symm/internal"
)

type ChartSettlement struct {
	TargetUnix float64
	Forecast   float64
	Actual     float64
}

type ChartEvents struct {
	EventAt        time.Time
	ForecastTarget float64
	Forecast       float64
	HasForecast    bool
	Settlements    []ChartSettlement
}

type forecastBucket struct {
	forecasts map[string]float64
}

type settlementBucket struct {
	forecastSum float64
	actualSum   float64
	symbols     map[string]struct{}
	published   bool
}

/*
Chart publishes cross-section means for the prediction dashboard. Forecast,
actual, and error share the same x: the forecast maturity unix second
(event time + horizon). Settlements accumulate through the maturity second and
publish once after it closes so ground truth and error do not revise in the
past as later symbols settle.
*/
type Chart struct {
	bus               *internal.Bus
	horizonSeconds    float64
	symbolForecast    map[string]int64
	forecastBuckets   map[int64]*forecastBucket
	settlementBuckets map[int64]*settlementBucket
}

func NewChart(bus *internal.Bus, horizon time.Duration) *Chart {
	horizonSeconds := horizon.Seconds()

	if horizonSeconds <= 0 {
		horizonSeconds = time.Minute.Seconds()
	}

	return &Chart{
		bus:               bus,
		horizonSeconds:    horizonSeconds,
		symbolForecast:    make(map[string]int64),
		forecastBuckets:   make(map[int64]*forecastBucket),
		settlementBuckets: make(map[int64]*settlementBucket),
	}
}

func (chart *Chart) Apply(symbol string, events ChartEvents) error {
	if events.HasForecast {
		if publishErr := chart.publishForecast(symbol, events); publishErr != nil {
			return publishErr
		}
	}

	for _, settlement := range events.Settlements {
		if accumulateErr := chart.accumulateSettlement(symbol, settlement); accumulateErr != nil {
			return accumulateErr
		}
	}

	return chart.flushReadySettlements(events.EventAt)
}

func (chart *Chart) publishForecast(symbol string, events ChartEvents) error {
	targetKey := int64(events.ForecastTarget)

	if targetKey <= 0 {
		return nil
	}

	chart.symbolForecast[symbol] = targetKey

	bucket := chart.forecastBuckets[targetKey]

	if bucket == nil {
		bucket = &forecastBucket{
			forecasts: make(map[string]float64),
		}
		chart.forecastBuckets[targetKey] = bucket
	}

	bucket.forecasts[symbol] = events.Forecast

	mean, count := chart.forecastBucketMean(bucket)

	if count == 0 {
		return nil
	}

	return chart.sendFrame(
		"prediction",
		float64(targetKey),
		mean,
		count,
	)
}

func (chart *Chart) forecastBucketMean(bucket *forecastBucket) (float64, int) {
	forecastSum := 0.0
	forecastCount := 0

	for _, forecast := range bucket.forecasts {
		forecastSum += forecast
		forecastCount++
	}

	if forecastCount == 0 {
		return 0, 0
	}

	return forecastSum / float64(forecastCount), forecastCount
}

func (chart *Chart) accumulateSettlement(
	symbol string,
	settlement ChartSettlement,
) error {
	targetKey := int64(settlement.TargetUnix)
	bucket, exists := chart.settlementBuckets[targetKey]

	if !exists {
		bucket = &settlementBucket{
			symbols: make(map[string]struct{}),
		}
		chart.settlementBuckets[targetKey] = bucket
	}

	if bucket.published {
		return nil
	}

	if _, seen := bucket.symbols[symbol]; seen {
		return nil
	}

	bucket.symbols[symbol] = struct{}{}
	bucket.forecastSum += settlement.Forecast
	bucket.actualSum += settlement.Actual

	return nil
}

func (chart *Chart) flushReadySettlements(eventAt time.Time) error {
	if eventAt.IsZero() {
		eventAt = time.Now()
	}

	nowUnix := eventAt.Unix()

	for targetKey, bucket := range chart.settlementBuckets {
		if bucket.published || len(bucket.symbols) == 0 {
			continue
		}

		if nowUnix <= targetKey {
			continue
		}

		if publishErr := chart.publishSettlementBucket(targetKey, bucket); publishErr != nil {
			return publishErr
		}

		bucket.published = true
	}

	return nil
}

func (chart *Chart) publishSettlementBucket(
	targetKey int64,
	bucket *settlementBucket,
) error {
	sampleCount := float64(len(bucket.symbols))
	meanForecast := bucket.forecastSum / sampleCount
	meanActual := bucket.actualSum / sampleCount
	meanError := math.Abs(meanActual - meanForecast)
	sampleCountInt := int(sampleCount)
	targetUnix := float64(targetKey)

	if publishErr := chart.sendFrame(
		"prediction",
		targetUnix,
		meanForecast,
		sampleCountInt,
	); publishErr != nil {
		return publishErr
	}

	if publishErr := chart.sendFrame(
		"actual",
		targetUnix,
		meanActual,
		sampleCountInt,
	); publishErr != nil {
		return publishErr
	}

	return chart.sendFrame(
		"error",
		targetUnix,
		meanError,
		sampleCountInt,
	)
}

func (chart *Chart) sendFrame(
	kind string,
	targetUnix float64,
	value float64,
	samples int,
) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}

	frame := map[string]any{
		"type":    "prediction",
		"chart":   "prediction",
		"kind":    kind,
		"x":       targetUnix,
		"value":   value,
		"horizon": chart.horizonSeconds,
	}

	if samples > 0 {
		frame["samples"] = samples
	}

	return chart.bus.Send(internal.ChannelUI, "prediction", frame)
}
