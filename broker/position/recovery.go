package position

import (
	"context"
	"slices"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/* Recovery reconstructs inventory from venue balances and complete trade facts. */
type Recovery struct {
	API   *websocket.API
	Price *broker.Price
}

func (recovery *Recovery) Recover(
	ctx context.Context,
	quote string,
	record func(kraken.ExecutionData) error,
) (map[string]*Regulator, error) {
	balances, err := recovery.API.Balance()

	if err != nil {
		return nil, errnie.Error(err)
	}
	history, err := recovery.API.TradesHistory()

	if err != nil {
		return nil, errnie.Error(err)
	}
	orders, err := recovery.API.OpenOrders()

	if err != nil {
		return nil, errnie.Error(err)
	}
	positions := make(map[string]*Regulator)

	for _, row := range balances.Data {
		asset := recovery.API.Normalizer().Name(row.Asset)
		amount := row.Balance

		if asset == quote || amount.Sign() <= 0 {
			continue
		}
		symbol := asset + "/" + quote
		position := NewRegulator(recovery.API, recovery.Price, symbol, record)
		position.Recovered = true
		position.Holding.Qty = amount
		position.Holding.SellableQty = amount
		position.Holding.Basis = nil
		positions[symbol] = position
		ledger, err := recovery.reconstruct(symbol, history.Trades)

		if err != nil {
			return positions, err
		}

		if ledger.Qty.Cmp(amount) != 0 {
			return positions, errnie.Error(errnie.Err(
				errnie.Conflict, "recovery: balance and trade quantity disagree for "+symbol, nil,
			))
		}
		position.Holding = ledger
		position.Holding.Status = types.OPEN
	}

	for identity, order := range orders.Open {
		if order.Description == nil {
			return positions, errnie.Error(errnie.Err(
				errnie.Validation, "recovery: working order description required", nil,
			))
		}
		symbol := recovery.API.Normalizer().Name(order.Description.Pair)
		position := positions[symbol]

		if position == nil {
			position = NewRegulator(recovery.API, recovery.Price, symbol, record)
			position.Recovered = true
			positions[symbol] = position
		}

		if position.pending != nil {
			return positions, errnie.Error(errnie.Err(
				errnie.Conflict, "recovery: multiple working orders for "+symbol, nil,
			))
		}

		if order.Volume == nil || order.VolumeExecuted == nil || order.Cost == nil || order.Fee == nil {
			return positions, errnie.Error(errnie.Err(
				errnie.Validation, "recovery: working order economics required", nil,
			))
		}
		position.pending = &spot.AddOrderRequest{
			ClOrdId:   order.ClOrdID,
			Pair:      symbol,
			Type:      order.Description.Type,
			OrderType: order.Description.OrderType,
			Volume:    order.Volume.String(),
		}
		position.result.ID = []string{identity}
		position.executed = kraken.ExecutionData{
			CumQty:      order.VolumeExecuted,
			CumCost:     order.Cost,
			FeeUsdEquiv: order.Fee,
		}
		position.Holding.Status = types.PENDING
	}
	for _, position := range positions {
		position.Guardian = NewGuardian(position)
		position.Guardian.Start(ctx)
	}
	return positions, nil
}

/* reconstruct reuses live fill accounting; it does not invent an originating intent. */
func (recovery *Recovery) reconstruct(
	symbol string,
	history map[string]spot.Trade,
) (*types.Holding, error) {
	trades := make([]spot.Trade, 0)

	for _, trade := range history {
		if recovery.API.Normalizer().Name(trade.Pair) != symbol {
			continue
		}

		if trade.Time == nil || trade.Volume == nil || trade.Cost == nil || trade.Fee == nil {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation, "recovery: incomplete trade economics for "+symbol, nil,
			))
		}
		trades = append(trades, trade)
	}
	slices.SortFunc(trades, func(left, right spot.Trade) int {
		return left.Time.Cmp(right.Time)
	})
	ledger := types.NewHolding(symbol)
	zero := decimal.NewFromInt64(0)
	previous := kraken.ExecutionData{CumQty: zero, CumCost: zero, FeeUsdEquiv: zero}

	for _, trade := range trades {
		if ledger.Qty.Sign() == 0 {
			ledger = types.NewHolding(symbol)
		}
		if err := recovery.Price.ApplyFill(ledger, kraken.ExecutionData{
			Symbol:      symbol,
			Side:        trade.Type,
			CumQty:      trade.Volume,
			CumCost:     trade.Cost,
			FeeUsdEquiv: trade.Fee,
			AvgPrice:    trade.Price,
			Timestamp:   time.Unix(trade.Time.Int64(), 0).UTC(),
		}, previous); err != nil {
			return nil, err
		}
	}
	return ledger, nil
}
