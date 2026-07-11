package manifold

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/adaptive"
)

/*
EpochTransform freezes per-epoch coordinate scales for conservative mapping.
*/
type EpochTransform struct {
	PriceScale float64
	SizeScale  float64
	Version    uint64
}

/*
CoordinateMapper maintains per-symbol adaptive scales and maps raw orders into
dimensionless physical coordinates.
*/
type CoordinateMapper struct {
	halflife     time.Duration
	epsilon      float64
	scaleVersion uint64
	logPrice     *adaptive.TimeElastic
	logSize      *adaptive.TimeElastic
	lifetime     *LifetimeEstimator
}

func NewCoordinateMapper(
	halflife time.Duration,
	epsilon float64,
	lifetime *LifetimeEstimator,
) *CoordinateMapper {
	return &CoordinateMapper{
		halflife: halflife,
		epsilon:  epsilon,
		logPrice: adaptive.NewTimeElastic(adaptive.TimeElasticConfig{Halflife: halflife, Epsilon: epsilon}),
		logSize:  adaptive.NewTimeElastic(adaptive.TimeElasticConfig{Halflife: halflife, Epsilon: epsilon}),
		lifetime: lifetime,
	}
}

func (mapper *CoordinateMapper) ScaleVersion() uint64 {
	return mapper.scaleVersion
}

/*
BeginEpoch observes all carriers once, freezes scales for the integration epoch,
and bumps ScaleVersion when baselines change.
*/
func (mapper *CoordinateMapper) BeginEpoch(
	orders []*PhysicalOrder,
	midPrice float64,
	at time.Time,
) (EpochTransform, bool) {
	if midPrice <= 0 || at.IsZero() {
		return EpochTransform{}, false
	}

	previousPrice := mapper.logPrice.Baseline()
	previousSize := mapper.logSize.Baseline()

	for _, order := range orders {
		if !mapper.observeOrder(order, midPrice, at) {
			return EpochTransform{}, false
		}
	}

	if !mapper.lifetime.Ready() {
		for _, order := range orders {
			mapper.lifetime.Censor(at.Sub(order.AddedAt))
		}
	}

	if !mapper.logPrice.Ready() || !mapper.logSize.Ready() || !mapper.lifetime.Ready() {
		return EpochTransform{}, false
	}

	priceScale := mapper.logPrice.Baseline() + mapper.epsilon
	sizeScale := mapper.logSize.Baseline() + mapper.epsilon

	if priceScale <= 0 || sizeScale <= 0 {
		return EpochTransform{}, false
	}

	if previousPrice > 0 && previousSize > 0 {
		if math.Abs(priceScale-previousPrice-mapper.epsilon) > mapper.epsilon ||
			math.Abs(sizeScale-previousSize-mapper.epsilon) > mapper.epsilon {
			mapper.scaleVersion++
		}
	}

	return EpochTransform{
		PriceScale: priceScale,
		SizeScale:  sizeScale,
		Version:    mapper.scaleVersion,
	}, true
}

func (mapper *CoordinateMapper) MapOrder(
	order *PhysicalOrder,
	midPrice float64,
	at time.Time,
	transform EpochTransform,
) (Coordinate, bool) {
	if order == nil || midPrice <= 0 || at.IsZero() || transform.PriceScale <= 0 || transform.SizeScale <= 0 {
		return Coordinate{}, false
	}

	logDisplacement := math.Log(order.LimitPrice / midPrice)
	logSize := math.Log1p(order.Quantity)
	age := at.Sub(order.AddedAt)

	if age < 0 {
		return Coordinate{}, false
	}

	return Coordinate{
		Price: logDisplacement / transform.PriceScale,
		Size:  logSize / transform.SizeScale,
		Age:   mapper.lifetime.SurvivalFraction(age),
	}, true
}

func (mapper *CoordinateMapper) UpdateVelocity(
	order *PhysicalOrder,
	previous Coordinate,
	next Coordinate,
	at time.Time,
) {
	if order == nil || at.IsZero() || order.MappedAt.IsZero() {
		return
	}

	deltaSeconds := at.Sub(order.MappedAt).Seconds()

	if deltaSeconds <= 0 {
		return
	}

	order.Velocity = Coordinate{
		Price: (next.Price - previous.Price) / deltaSeconds,
		Size:  (next.Size - previous.Size) / deltaSeconds,
		Age:   (next.Age - previous.Age) / deltaSeconds,
	}
}

func (mapper *CoordinateMapper) observeOrder(
	order *PhysicalOrder,
	midPrice float64,
	at time.Time,
) bool {
	if order == nil {
		return false
	}

	logDisplacement := math.Abs(math.Log(order.LimitPrice / midPrice))
	logSize := math.Log1p(order.Quantity)

	if _, err := mapper.logPrice.Measure(adaptive.TimedValue{Value: logDisplacement, At: at}); err != nil {
		errnie.Error(err)

		return false
	}

	if _, err := mapper.logSize.Measure(adaptive.TimedValue{Value: logSize, At: at}); err != nil {
		errnie.Error(err)

		return false
	}

	return true
}
