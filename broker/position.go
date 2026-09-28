package broker

import (
	"encoding/json"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
)

type ExecutionSnapshot struct {
	ExecutedPrice  *decimal.Decimal `json:"executedPrice,omitempty"`
	ExecutedVolume *decimal.Decimal `json:"executedVolume,omitempty"`
	Fee            *decimal.Decimal `json:"fee,omitempty"`
}

/*
Position is one leg of an open hedge.
*/
type Position struct {
	PositionID    string                `json:"positionId"`
	EntryOrder    *spot.AddOrderRequest `json:"entryOrder"`
	ExitOrder     *spot.AddOrderRequest `json:"exitOrder"`
	EntryResponse *spot.AddOrderResult  `json:"entryResponse"`
	ExitResponse  *spot.AddOrderResult  `json:"exitResponse"`
	execution     atomic.Pointer[ExecutionSnapshot]
	EntryAt       time.Time `json:"entryAt"`
}

func NewPosition(
	entryOrder *spot.AddOrderRequest,
	exitOrder *spot.AddOrderRequest,
) *Position {
	posID := uuid.New().String()

	if entryOrder != nil && entryOrder.ClOrdId == "" {
		entryOrder.ClOrdId = posID
	}

	return &Position{
		PositionID: posID,
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

	position.execution.Store(&ExecutionSnapshot{
		ExecutedPrice:  price,
		ExecutedVolume: volume,
		Fee:            fee,
	})
}

func (position *Position) Execution() *ExecutionSnapshot {
	if position == nil {
		return nil
	}

	return position.execution.Load()
}

func (position *Position) ExecutedPrice() *decimal.Decimal {
	if snap := position.Execution(); snap != nil {
		return snap.ExecutedPrice
	}

	return nil
}

func (position *Position) ExecutedVolume() *decimal.Decimal {
	if snap := position.Execution(); snap != nil {
		return snap.ExecutedVolume
	}

	return nil
}

func (position *Position) Fee() *decimal.Decimal {
	if snap := position.Execution(); snap != nil {
		return snap.Fee
	}

	return nil
}

func (position *Position) Price() *decimal.Decimal {
	if position == nil {
		return nil
	}

	if snap := position.Execution(); snap != nil && snap.ExecutedPrice != nil && snap.ExecutedPrice.Sign() > 0 {
		return snap.ExecutedPrice
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

	if snap := position.Execution(); snap != nil && snap.ExecutedVolume != nil && snap.ExecutedVolume.Sign() > 0 {
		return snap.ExecutedVolume
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

	if position.EntryOrder != nil && position.EntryOrder.ClOrdId != "" {
		return position.EntryOrder.ClOrdId
	}

	return position.PositionID
}

type positionJSON struct {
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

func (position *Position) MarshalJSON() ([]byte, error) {
	if position == nil {
		return []byte("null"), nil
	}

	snap := position.Execution()
	var executedPrice, executedVolume, fee *decimal.Decimal

	if snap != nil {
		executedPrice = snap.ExecutedPrice
		executedVolume = snap.ExecutedVolume
		fee = snap.Fee
	}

	return json.Marshal(positionJSON{
		PositionID:     position.PositionID,
		EntryOrder:     position.EntryOrder,
		ExitOrder:      position.ExitOrder,
		EntryResponse:  position.EntryResponse,
		ExitResponse:   position.ExitResponse,
		ExecutedPrice:  executedPrice,
		ExecutedVolume: executedVolume,
		Fee:            fee,
		EntryAt:        position.EntryAt,
	})
}

func (position *Position) UnmarshalJSON(data []byte) error {
	var row positionJSON

	if err := json.Unmarshal(data, &row); err != nil {
		return err
	}

	position.PositionID = row.PositionID
	position.EntryOrder = row.EntryOrder
	position.ExitOrder = row.ExitOrder
	position.EntryResponse = row.EntryResponse
	position.ExitResponse = row.ExitResponse
	position.EntryAt = row.EntryAt

	if row.ExecutedPrice != nil || row.ExecutedVolume != nil || row.Fee != nil {
		position.SetFill(row.ExecutedPrice, row.ExecutedVolume, row.Fee)
	}

	return nil
}
