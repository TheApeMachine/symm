package broker

import (
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
)

type Position struct {
	entryOrders []*spot.OrderRequest
	exitOrders  []*spot.OrderRequest
	fills       []*kraken.ExecutionData
	qty         *decimal.Decimal
	entryPrice  *decimal.Decimal
	exitPrice   *decimal.Decimal
	entryFee    *decimal.Decimal
	exitFee     *decimal.Decimal
	mark        *decimal.Decimal
}

func NewPosition() *Position {
	return &Position{
		entryOrders: make([]*spot.OrderRequest, 0),
		exitOrders:  make([]*spot.OrderRequest, 0),
		fills:       make([]*kraken.ExecutionData, 0),
	}
}

func (position *Position) Enter(symbol string, qty *decimal.Decimal) error {
	position.qty = qty

	position.entryOrders = append(position.entryOrders, &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "limit",
		Type:      "buy",
		Volume:    qty.String(),
	})

	return nil
}

func (position *Position) Exit(symbol string) error {
	position.exitOrders = append(position.exitOrders, &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "limit",
		Type:      "sell",
		Volume:    position.qty.String(),
	})

	return nil
}

func (position *Position) Apply(execution kraken.ExecutionData) {
	position.fills = append(position.fills, &execution)

	switch execution.ExecType {
	case "Trade":
	case "pending_new":
	case "new":
	case "trade":
	case "filled":
	case "iceberg_refill":
	case "canceled":
	case "expired":
	case "amended":
	case "restated":
	case "status":
	}
}
