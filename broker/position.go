package broker

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
)

/*
Position is one leg of an open hedge.
*/
type Position struct {
	PositionID     string                `json:"positionId"`
	EntryOrder     *spot.AddOrderRequest `json:"entryOrder"`
	ExitOrder      *spot.AddOrderRequest `json:"exitOrder"`
	EntryResponse  *spot.AddOrderResult  `json:"entryResponse"`
	ExitResponse   *spot.AddOrderResult  `json:"exitResponse"`
	ExecutedPrice  *decimal.Decimal      `json:"executedPrice,omitempty"`
	ExecutedVolume *decimal.Decimal      `json:"executedVolume,omitempty"`
	Fee            *decimal.Decimal      `json:"fee,omitempty"`
	EntryAt        time.Time             `json:"entryAt"`
}

func NewPosition(
	entryOrder *spot.AddOrderRequest,
	exitOrder *spot.AddOrderRequest,
) *Position {
	return &Position{
		EntryOrder: entryOrder,
		ExitOrder:  exitOrder,
		EntryAt:    time.Now(),
	}
}

func (position *Position) AddEntryResponse(response *spot.AddOrderResult) {
	position.EntryResponse = response
}

func (position *Position) AddExitResponse(response *spot.AddOrderResult) {
	position.ExitResponse = response
}

func (position *Position) SetFill(price, volume, fee *decimal.Decimal) {
	if position == nil {
		return
	}

	position.ExecutedPrice = price
	position.ExecutedVolume = volume
	position.Fee = fee
}

func (position *Position) Price() *decimal.Decimal {
	if position == nil {
		return nil
	}

	if position.ExecutedPrice != nil && position.ExecutedPrice.Sign() > 0 {
		return position.ExecutedPrice
	}

	if position.EntryOrder == nil || position.EntryOrder.Price == "" {
		return nil
	}

	price, err := decimal.NewFromString(position.EntryOrder.Price)

	if err != nil {
		return nil
	}

	return price
}

func (position *Position) Volume() *decimal.Decimal {
	if position == nil {
		return nil
	}

	if position.ExecutedVolume != nil && position.ExecutedVolume.Sign() > 0 {
		return position.ExecutedVolume
	}

	if position.EntryOrder == nil || position.EntryOrder.Volume == "" {
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
