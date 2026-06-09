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
	ForecastTarget float64
	Forecast       float64
	HasForecast    bool
	Settlements    []ChartSettlement
}

type settlementBucket struct {
	forecastSum float64
	actualSum   float64
	symbols     map[string]struct{}
}

/*
Chart turns per-pair normalized predictions and ground truth into cross-section
means for the dashboard. Live frames publish the latest mean forecast; catch-up
frames publish mean forecast, mean actual, and their difference at maturity.
*/
type Chart struct {
	bus                 *internal.Bus
	horizonSeconds      float64
	forecasts           map[string]float64
	forecastTargets     map[string]float64
	settlements         map[int64]*settlementBucket
	lastLiveTarget      float64
	lastLiveSymbolCount int
}

func NewChart(bus *internal.Bus, horizon time.Duration) *Chart {
	horizonSeconds := horizon.Seconds()

	if horizonSeconds <= 0 {
		horizonSeconds = time.Minute.Seconds()
	}

	return &Chart{
		bus:             bus,
		horizonSeconds:  horizonSeconds,
		forecasts:       make(map[string]float64),
		forecastTargets: make(map[string]float64),
		settlements:     make(map[int64]*settlementBucket),
	}
}

func (chart *Chart) Apply(symbol string, events ChartEvents) error {
	if events.HasForecast {
		chart.forecasts[symbol] = events.Forecast
		chart.forecastTargets[symbol] = events.ForecastTarget

		if publishErr := chart.publishLatestPrediction(); publishErr != nil {
			return publishErr
		}
	}

	for _, settlement := range events.Settlements {
		if publishErr := chart.publishCatchUp(symbol, settlement); publishErr != nil {
			return publishErr
		}
	}

	return nil
}

func (chart *Chart) publishLatestPrediction() error {
	forecastSum := 0.0
	forecastCount := 0
	rightEdge := 0.0

	for symbol, forecast := range chart.forecasts {
		forecastSum += forecast
		forecastCount++

		targetUnix := chart.forecastTargets[symbol]

		if targetUnix > rightEdge {
			rightEdge = targetUnix
		}
	}

	if forecastCount == 0 || rightEdge <= 0 {
		return nil
	}

	if rightEdge == chart.lastLiveTarget &&
		forecastCount == chart.lastLiveSymbolCount {
		return nil
	}

	chart.lastLiveTarget = rightEdge
	chart.lastLiveSymbolCount = forecastCount

	return chart.sendFrame(
		"prediction",
		rightEdge,
		forecastSum/float64(forecastCount),
	)
}

func (chart *Chart) publishCatchUp(
	symbol string,
	settlement ChartSettlement,
) error {
	targetKey := int64(settlement.TargetUnix)
	bucket, exists := chart.settlements[targetKey]

	if !exists {
		bucket = &settlementBucket{
			symbols: make(map[string]struct{}),
		}
		chart.settlements[targetKey] = bucket
	}

	if _, seen := bucket.symbols[symbol]; seen {
		return nil
	}

	bucket.symbols[symbol] = struct{}{}
	bucket.forecastSum += settlement.Forecast
	bucket.actualSum += settlement.Actual

	sampleCount := float64(len(bucket.symbols))
	meanForecast := bucket.forecastSum / sampleCount
	meanActual := bucket.actualSum / sampleCount
	meanError := math.Abs(meanActual - meanForecast)

	if publishErr := chart.sendFrame(
		"prediction",
		settlement.TargetUnix,
		meanForecast,
	); publishErr != nil {
		return publishErr
	}

	if publishErr := chart.sendFrame(
		"actual",
		settlement.TargetUnix,
		meanActual,
	); publishErr != nil {
		return publishErr
	}

	return chart.sendFrame(
		"error",
		settlement.TargetUnix,
		meanError,
	)
}

func (chart *Chart) sendFrame(
	kind string,
	targetUnix float64,
	value float64,
) error {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}

	return chart.bus.Send("ui", "prediction", map[string]any{
		"chart":   "prediction",
		"kind":    kind,
		"x":       targetUnix,
		"value":   value,
		"horizon": chart.horizonSeconds,
	})
}
