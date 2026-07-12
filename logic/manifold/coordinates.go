package manifold

import (
	"math"
	"time"
)

/*
EpochTransform freezes per-epoch coordinate scales for conservative mapping.
*/
type EpochTransform struct {
	PriceScale   float64
	SizeLocation float64
	SizeScale    float64
	Version      uint64
}

/*
CoordinateMapper maintains per-symbol epoch statistics and maps raw orders into
dimensionless physical coordinates.
*/
type CoordinateMapper struct {
	epsilon      float64
	scaleVersion uint64
	priceScale   *EpochScale
	sizeLocation *EpochScale
	sizeScale    *EpochScale
	lifetime     *LifetimeEstimator
}

func NewCoordinateMapper(
	halflife time.Duration,
	epsilon float64,
	lifetime *LifetimeEstimator,
) *CoordinateMapper {
	return &CoordinateMapper{
		epsilon:      epsilon,
		priceScale:   NewEpochScale(halflife),
		sizeLocation: NewEpochScale(halflife),
		sizeScale:    NewEpochScale(halflife),
		lifetime:     lifetime,
	}
}

func (mapper *CoordinateMapper) ScaleVersion() uint64 {
	return mapper.scaleVersion
}

/*
BeginEpoch computes one complete population statistic before updating temporal
scales, so renaming or permuting order IDs cannot alter the transform.
*/
func (mapper *CoordinateMapper) BeginEpoch(
	orders []*PhysicalOrder,
	midPrice float64,
	at time.Time,
) (EpochTransform, bool) {
	statistics := CoordinateStatistics{}

	if at.IsZero() || mapper.lifetime == nil || !mapper.lifetime.Ready() {
		return EpochTransform{}, false
	}

	mapper.lifetime.prepare()

	if !statistics.Measure(orders, midPrice, mapper.epsilon) {
		return EpochTransform{}, false
	}

	previous := mapper.transform()
	priceScale, priceReady := mapper.priceScale.Update(statistics.PriceScale, at)
	sizeLocation, locationReady := mapper.sizeLocation.Update(statistics.SizeLocation, at)
	sizeScale, sizeReady := mapper.sizeScale.Update(statistics.SizeScale, at)

	if !priceReady || !locationReady || !sizeReady {
		return EpochTransform{}, false
	}

	if previous.PriceScale > 0 && mapper.changed(previous, priceScale, sizeLocation, sizeScale) {
		mapper.scaleVersion++
	}

	return EpochTransform{
		PriceScale:   priceScale,
		SizeLocation: sizeLocation,
		SizeScale:    sizeScale,
		Version:      mapper.scaleVersion,
	}, true
}

func (mapper *CoordinateMapper) MapOrder(
	order *PhysicalOrder,
	midPrice float64,
	at time.Time,
	transform EpochTransform,
) (Coordinate, bool) {
	if order == nil || order.LimitPrice <= 0 || order.Quantity <= 0 ||
		midPrice <= 0 || at.IsZero() || transform.PriceScale <= 0 ||
		transform.SizeScale <= 0 {
		return Coordinate{}, false
	}

	age := at.Sub(order.AddedAt)

	if age < 0 {
		return Coordinate{}, false
	}

	return Coordinate{
		Price: math.Log(order.LimitPrice/midPrice) / transform.PriceScale,
		Size:  (math.Log1p(order.Quantity) - transform.SizeLocation) / transform.SizeScale,
		Age:   mapper.lifetime.CDF(age),
	}, true
}

func (mapper *CoordinateMapper) UpdateVelocity(
	order *PhysicalOrder,
	previous Coordinate,
	next Coordinate,
	at time.Time,
	transform EpochTransform,
	midPrice float64,
) {
	if order == nil || at.IsZero() || order.MappedAt.IsZero() {
		return
	}

	if order.ScaleVersion != transform.Version || order.ReferenceMid != midPrice {
		order.Velocity = Coordinate{}
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

func (mapper *CoordinateMapper) transform() EpochTransform {
	return EpochTransform{
		PriceScale:   mapper.priceScale.Value(),
		SizeLocation: mapper.sizeLocation.Value(),
		SizeScale:    mapper.sizeScale.Value(),
		Version:      mapper.scaleVersion,
	}
}

func (mapper *CoordinateMapper) changed(
	previous EpochTransform,
	priceScale float64,
	sizeLocation float64,
	sizeScale float64,
) bool {
	return math.Abs(previous.PriceScale-priceScale) > mapper.epsilon ||
		math.Abs(previous.SizeLocation-sizeLocation) > mapper.epsilon ||
		math.Abs(previous.SizeScale-sizeScale) > mapper.epsilon
}
