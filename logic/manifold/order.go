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
	OrderID    string
	Side       OrderSide
	LimitPrice float64
	Quantity   float64
	AddedAt    time.Time
	UpdatedAt  time.Time
	MappedAt   time.Time
	Coordinate Coordinate
	Velocity   Coordinate
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
PopulationAccounting tracks exact base-quantity ledger deltas for replay identity.
*/
type PopulationAccounting struct {
	Initial   float64
	Added     float64
	Cancelled float64
	Filled    float64
	Amended   float64
}

func (accounting PopulationAccounting) Final() float64 {
	return accounting.Initial +
		accounting.Added -
		accounting.Cancelled -
		accounting.Filled +
		accounting.Amended
}
