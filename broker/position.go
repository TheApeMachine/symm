package broker

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
)

/*
Position is one leg of an open hedge.
*/
type Position struct {
	PositionID    string                `json:"positionId"`
	EntryOrder    *spot.AddOrderRequest `json:"entryOrder"`
	ExitOrder     *spot.AddOrderRequest `json:"exitOrder"`
	EntryResponse *spot.AddOrderResult  `json:"entryResponse"`
	ExitResponse  *spot.AddOrderResult  `json:"exitResponse"`
}

func NewPosition(
	entryOrder *spot.AddOrderRequest,
	exitOrder *spot.AddOrderRequest,
) *Position {
	return &Position{
		EntryOrder: entryOrder,
		ExitOrder:  exitOrder,
	}
}

func (position *Position) AddEntryResponse(response *spot.AddOrderResult) {
	position.EntryResponse = response
}

func (position *Position) AddExitResponse(response *spot.AddOrderResult) {
	position.ExitResponse = response
}

func (position *Position) Price() *decimal.Decimal {
	if position == nil || position.EntryOrder == nil || position.EntryOrder.Price == "" {
		return nil
	}

	price, err := decimal.NewFromString(position.EntryOrder.Price)

	if err != nil {
		return nil
	}

	return price
}

func (position *Position) Volume() *decimal.Decimal {
	if position == nil || position.EntryOrder == nil || position.EntryOrder.Volume == "" {
		return nil
	}

	volume, err := decimal.NewFromString(position.EntryOrder.Volume)

	if err != nil {
		return nil
	}

	return volume
}

func (position *Position) OrderID() string {
	if position == nil {
		return ""
	}

	if position.EntryResponse != nil && len(position.EntryResponse.ID) > 0 {
		return position.EntryResponse.ID[0]
	}

	return position.PositionID
}

