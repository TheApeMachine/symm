package manifold

import (
	"math"
	"sort"

	pmanifold "github.com/theapemachine/nomagique/physics/manifold"
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
Cohort conservatively aggregates spatially local orders while preserving mass,
centroid, mean velocity, and second central velocity moment.
*/
type Cohort struct {
	Side         OrderSide
	Mass         float64
	MassRoundoff float64
	Centroid     Coordinate
	Velocity     Coordinate
	SecondMoment Coordinate
	CrossMoment  VelocityCross
	Count        int
}

type cohortCell struct {
	side OrderSide
	x    uint32
	y    uint32
	z    uint32
}

/*
CohortBuilder groups only carriers that occupy the same physical grid cell.
*/
type CohortBuilder struct {
	config *pmanifold.Config
}

func NewCohortBuilder(config *pmanifold.Config) *CohortBuilder {
	return &CohortBuilder{config: config}
}

func (builder *CohortBuilder) Build(orders []*PhysicalOrder) []Cohort {
	if len(orders) == 0 || builder.config == nil {
		return nil
	}

	buckets := map[cohortCell][]*PhysicalOrder{}

	for _, order := range orders {
		cell := builder.cell(order)
		buckets[cell] = append(buckets[cell], order)
	}

	keys := make([]cohortCell, 0, len(buckets))

	for key := range buckets {
		keys = append(keys, key)
	}

	sort.Slice(keys, func(left, right int) bool {
		if keys[left].side != keys[right].side {
			return keys[left].side < keys[right].side
		}

		if keys[left].x != keys[right].x {
			return keys[left].x < keys[right].x
		}

		if keys[left].y != keys[right].y {
			return keys[left].y < keys[right].y
		}

		return keys[left].z < keys[right].z
	})

	cohorts := make([]Cohort, 0, len(keys))

	for _, key := range keys {
		bucket := buckets[key]
		mass := roundedQuantity{}

		for _, order := range bucket {
			mass = mass.Add(roundedQuantity{value: order.Quantity})
		}

		if mass.value > 0 {
			cohort := builder.cohort(key.side, bucket, mass.value)
			cohort.MassRoundoff = mass.roundoff
			cohorts = append(cohorts, cohort)
		}
	}

	return cohorts
}

func (builder *CohortBuilder) cell(order *PhysicalOrder) cohortCell {
	return cohortCell{
		side: order.Side,
		x: builder.centeredCell(
			order.Coordinate.Price,
			builder.config.DomainX,
			builder.config.GridX,
		),
		y: builder.centeredCell(
			order.Coordinate.Size,
			builder.config.DomainY,
			builder.config.GridY,
		),
		z: builder.unitCell(order.Coordinate.Age, builder.config.GridZ),
	}
}

func (builder *CohortBuilder) centeredCell(value float64, domain float64, grid uint32) uint32 {
	if grid == 0 || domain <= 0 {
		return 0
	}

	normalized := min(max(value/domain+0.5, 0), 1)
	return min(uint32(math.Floor(normalized*float64(grid))), grid-1)
}

func (builder *CohortBuilder) unitCell(value float64, grid uint32) uint32 {
	if grid == 0 {
		return 0
	}

	normalized := min(max(value, 0), 1)
	return min(uint32(math.Floor(normalized*float64(grid))), grid-1)
}

func (builder *CohortBuilder) cohort(
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
		deltaPrice := order.Velocity.Price - velocity.Price
		deltaSize := order.Velocity.Size - velocity.Size
		deltaAge := order.Velocity.Age - velocity.Age
		secondMoment.Price += weight * deltaPrice * deltaPrice
		secondMoment.Size += weight * deltaSize * deltaSize
		secondMoment.Age += weight * deltaAge * deltaAge
		crossMoment.PriceSize += weight * deltaPrice * deltaSize
		crossMoment.PriceAge += weight * deltaPrice * deltaAge
		crossMoment.SizeAge += weight * deltaSize * deltaAge
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
