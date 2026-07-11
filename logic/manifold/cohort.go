package manifold

import (
	"math"
	"sort"
)

/*
VelocityCross holds the off-diagonal second central velocity moments.
*/
type VelocityCross struct {
	PriceSize float64
	PriceAge  float64
	SizeAge   float64
}

/*
Cohort conservatively aggregates orders while preserving mass, centroid,
mean velocity, and second central velocity moment.
*/
type Cohort struct {
	Side         OrderSide
	Mass         float64
	Centroid     Coordinate
	Velocity     Coordinate
	SecondMoment Coordinate
	CrossMoment  VelocityCross
	Count        int
}

/*
CohortBuilder groups mapped carriers into deposition cohorts.
*/
type CohortBuilder struct {
	maxCohortOrders int
}

func NewCohortBuilder(maxCohortOrders int) *CohortBuilder {
	if maxCohortOrders <= 0 {
		maxCohortOrders = 64
	}

	return &CohortBuilder{maxCohortOrders: maxCohortOrders}
}

func (builder *CohortBuilder) Build(orders []*PhysicalOrder) []Cohort {
	if len(orders) == 0 {
		return nil
	}

	bids := make([]*PhysicalOrder, 0, len(orders))
	asks := make([]*PhysicalOrder, 0, len(orders))

	for _, order := range orders {
		if order.Side == OrderSideBid {
			bids = append(bids, order)
			continue
		}

		asks = append(asks, order)
	}

	cohorts := make([]Cohort, 0, 2)

	if len(bids) > 0 {
		cohorts = append(cohorts, builder.buildSide(OrderSideBid, bids)...)
	}

	if len(asks) > 0 {
		cohorts = append(cohorts, builder.buildSide(OrderSideAsk, asks)...)
	}

	return cohorts
}

func (builder *CohortBuilder) buildSide(side OrderSide, orders []*PhysicalOrder) []Cohort {
	totalMass := 0.0

	for _, order := range orders {
		totalMass += order.Quantity
	}

	if totalMass <= 0 {
		return nil
	}

	if len(orders) <= builder.maxCohortOrders {
		return []Cohort{builder.cohortFromOrders(side, orders, totalMass)}
	}

	buckets := map[int][]*PhysicalOrder{}

	for _, order := range orders {
		key := builder.bucketKey(side, order.Coordinate)
		buckets[key] = append(buckets[key], order)
	}

	keys := make([]int, 0, len(buckets))

	for key := range buckets {
		keys = append(keys, key)
	}

	sort.Ints(keys)

	cohorts := make([]Cohort, 0, len(keys))

	for _, key := range keys {
		bucket := buckets[key]
		bucketMass := 0.0

		for _, order := range bucket {
			bucketMass += order.Quantity
		}

		cohorts = append(cohorts, builder.cohortFromOrders(side, bucket, bucketMass))
	}

	return cohorts
}

func (builder *CohortBuilder) bucketKey(side OrderSide, coordinate Coordinate) int {
	sideBit := 0

	if side == OrderSideAsk {
		sideBit = 1
	}

	priceBin := int(math.Floor(coordinate.Price * 8))
	sizeBin := int(math.Floor(coordinate.Size * 8))
	ageBin := int(math.Floor(coordinate.Age * 8))

	return sideBit*100000000 + priceBin*10000 + sizeBin*100 + ageBin
}

func (builder *CohortBuilder) cohortFromOrders(
	side OrderSide,
	orders []*PhysicalOrder,
	totalMass float64,
) Cohort {
	centroid := Coordinate{}
	velocity := Coordinate{}
	secondMoment := Coordinate{}
	crossMoment := VelocityCross{}

	for _, order := range orders {
		weight := order.Quantity / totalMass
		centroid.Price += weight * order.Coordinate.Price
		centroid.Size += weight * order.Coordinate.Size
		centroid.Age += weight * order.Coordinate.Age
		velocity.Price += weight * order.Velocity.Price
		velocity.Size += weight * order.Velocity.Size
		velocity.Age += weight * order.Velocity.Age
	}

	for _, order := range orders {
		weight := order.Quantity / totalMass
		deltaVelPrice := order.Velocity.Price - velocity.Price
		deltaVelSize := order.Velocity.Size - velocity.Size
		deltaVelAge := order.Velocity.Age - velocity.Age

		secondMoment.Price += weight * deltaVelPrice * deltaVelPrice
		secondMoment.Size += weight * deltaVelSize * deltaVelSize
		secondMoment.Age += weight * deltaVelAge * deltaVelAge
		crossMoment.PriceSize += weight * deltaVelPrice * deltaVelSize
		crossMoment.PriceAge += weight * deltaVelPrice * deltaVelAge
		crossMoment.SizeAge += weight * deltaVelSize * deltaVelAge
	}

	return Cohort{
		Side:         side,
		Mass:         totalMass,
		Centroid:     centroid,
		Velocity:     velocity,
		SecondMoment: secondMoment,
		CrossMoment:  crossMoment,
		Count:        len(orders),
	}
}
