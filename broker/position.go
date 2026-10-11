package broker

import (
	"strings"

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
	volume := "0"

	if position.qty != nil {
		volume = position.qty.String()
	}

	position.exitOrders = append(position.exitOrders, &spot.OrderRequest{
		ClOrdId:   uuid.NewString(),
		OrderType: "limit",
		Type:      "sell",
		Volume:    volume,
	})

	return nil
}

func (position *Position) Qty() *decimal.Decimal {
	return position.qty
}

func (position *Position) EntryPrice() *decimal.Decimal {
	return position.entryPrice
}

func (position *Position) ExitPrice() *decimal.Decimal {
	return position.exitPrice
}

func (position *Position) Closed() bool {
	return position.qty == nil || position.qty.Sign() <= 0
}

func (position *Position) Apply(execution kraken.ExecutionData) {
	position.fills = append(position.fills, &execution)

	if execution.LastQty == nil || execution.LastQty.Sign() <= 0 {
		return
	}

	side := strings.ToLower(execution.Side)

	if side == "buy" {
		if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
			position.qty = execution.CumQty
		}

		if execution.CumQty == nil || execution.CumQty.Sign() <= 0 {
			if position.qty == nil {
				position.qty = execution.LastQty
			}

			if position.qty != nil && position.qty != execution.LastQty {
				position.qty = position.qty.Add(execution.LastQty)
			}
		}

		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.entryPrice = execution.AvgPrice
		}

		if (execution.AvgPrice == nil || execution.AvgPrice.Sign() <= 0) && execution.LastPrice != nil {
			position.entryPrice = execution.LastPrice
		}

		if execution.FeeUsdEquiv != nil {
			if position.entryFee == nil {
				position.entryFee = execution.FeeUsdEquiv
			}

			if position.entryFee != nil && position.entryFee != execution.FeeUsdEquiv {
				position.entryFee = position.entryFee.Add(execution.FeeUsdEquiv)
			}
		}

		return
	}

	if side == "sell" {
		if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
			position.exitPrice = execution.AvgPrice
		}

		if (execution.AvgPrice == nil || execution.AvgPrice.Sign() <= 0) && execution.LastPrice != nil {
			position.exitPrice = execution.LastPrice
		}

		if execution.FeeUsdEquiv != nil {
			if position.exitFee == nil {
				position.exitFee = execution.FeeUsdEquiv
			}

			if position.exitFee != nil && position.exitFee != execution.FeeUsdEquiv {
				position.exitFee = position.exitFee.Add(execution.FeeUsdEquiv)
			}
		}

		if position.qty != nil {
			position.qty = position.qty.Sub(execution.LastQty)

			if position.qty.Sign() <= 0 {
				position.qty = decimal.NewFromFloat64(0)
			}
		}
	}
}
