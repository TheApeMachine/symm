package manifold

import (
	"time"
)

/*
OrderSide is the signed price-axis orientation: bid orders sit below mid (-1),
ask orders sit above mid (+1).
*/
type OrderSide int8

const (
	OrderSideBid OrderSide = -1
	OrderSideAsk OrderSide = 1
)

/*
PhysicalOrder is one visible L3 order carrier with exact exchange identity.
*/
type PhysicalOrder struct {
	OrderID      string
	Side         OrderSide
	LimitPrice   float64
	Quantity     float64
	AddedAt      time.Time
	UpdatedAt    time.Time
	MappedAt     time.Time
	ScaleVersion uint64
	ReferenceMid float64
	Coordinate   Coordinate
	Velocity     Coordinate
}

/*
Coordinate is a dimensionless position in the price/size/age field.
*/
type Coordinate struct {
	Price float64
	Size  float64
	Age   float64
}

/*
PopulationAccounting reconciles quantities across complete SDK book reads.
Removed is deliberately neutral because a book cannot prove whether an absent
order was cancelled, executed, or moved outside the subscribed depth.
*/
type PopulationAccounting struct {
	Initial  float64
	Added    float64
	Removed  float64
	Amended  float64
	roundoff populationRoundoff
}

type populationRoundoff struct {
	Initial float64
	Added   float64
	Removed float64
	Amended float64
}

func (accounting *PopulationAccounting) recordInitial(quantity float64) {
	accounting.record(&accounting.Initial, &accounting.roundoff.Initial, roundedQuantity{value: quantity})
}

func (accounting *PopulationAccounting) recordAdded(quantity float64) {
	accounting.record(&accounting.Added, &accounting.roundoff.Added, roundedQuantity{value: quantity})
}

/*
recordRemoved retains quantity absent from a complete book when the book alone
cannot distinguish cancellation from execution.
*/
func (accounting *PopulationAccounting) recordRemoved(quantity float64) {
	accounting.record(&accounting.Removed, &accounting.roundoff.Removed, roundedQuantity{value: quantity})
}

func (accounting *PopulationAccounting) recordAmended(previous, current float64) {
	change := roundedQuantity{value: current}.Subtract(roundedQuantity{value: previous})
	accounting.record(&accounting.Amended, &accounting.roundoff.Amended, change)
}

func (accounting *PopulationAccounting) record(
	value *float64,
	roundoff *float64,
	change roundedQuantity,
) {
	result := roundedQuantity{value: *value, roundoff: *roundoff}.Add(change)
	*value = result.value
	*roundoff = result.roundoff
}

func (accounting PopulationAccounting) roundedFinal() roundedQuantity {
	quantity := roundedQuantity{value: accounting.Initial, roundoff: accounting.roundoff.Initial}
	quantity = quantity.Add(roundedQuantity{value: accounting.Added, roundoff: accounting.roundoff.Added})
	quantity = quantity.Subtract(roundedQuantity{value: accounting.Removed, roundoff: accounting.roundoff.Removed})
	quantity = quantity.Add(roundedQuantity{value: accounting.Amended, roundoff: accounting.roundoff.Amended})

	return quantity
}

func (accounting PopulationAccounting) Final() float64 {
	return accounting.roundedFinal().value
}
