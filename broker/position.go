package broker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
	"github.com/theapemachine/symm/utils"
)

/*
Position is one lot shell owned and event-routed by Desk. Order correlation uses
each decision's client order ID, then the exchange order ID returned by REST.
*/
type Position struct {
	mu             sync.RWMutex
	ctx            context.Context
	cancel         context.CancelFunc
	api            *websocket.API
	ui             chan []byte
	instrument     *Instrument
	price          *Price
	balance        *Balance
	pair           kraken.InstrumentPair
	marked         bool
	entryFeeActual bool
	executionFees  map[string]struct{}
	ID             string                `json:"id"`
	EntryOrderID   string                `json:"entry_order_id,omitempty"`
	ExitOrderID    string                `json:"exit_order_id,omitempty"`
	Status         types.Status          `json:"status"`
	EntryOrder     *spot.AddOrderRequest `json:"entry_order"`
	ExitOrder      *spot.AddOrderRequest `json:"exit_order"`
	Holding        *types.Holding        `json:"holding"`
}

/*
NewPosition constructs one desk-owned lot shell.
*/
func NewPosition(
	ctx context.Context,
	api *websocket.API,
	ui chan []byte,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair kraken.InstrumentPair,
	decision types.Decision,
) *Position {
	errnie.Info("creating position for: " + pair.Symbol)
	ctx, cancel := context.WithCancel(ctx)

	holding := types.NewHolding(
		ctx,
		pair.Symbol,
		decision.ProposedQuantity,
		decision.Mark,
		decision.ReservationID,
		decision.Opportunity,
	)
	holding.EntryPrice = decision.EntryPrice.Copy()
	holding.EntryFee = decision.EntryFee.Copy()

	position := &Position{
		ctx:           ctx,
		cancel:        cancel,
		Status:        types.INITIALIZING,
		api:           api,
		ui:            ui,
		instrument:    instrument,
		price:         price,
		balance:       balance,
		pair:          pair,
		executionFees: make(map[string]struct{}),
		ID:            decision.ID,
		EntryOrder: &spot.AddOrderRequest{
			ClOrdId:   decision.ID,
			Type:      "buy",
			OrderType: "market",
			Volume:    decision.ProposedQuantity.String(),
			Pair:      pair.Symbol,
		},
		ExitOrder: &spot.AddOrderRequest{
			ClOrdId:   decision.ID,
			Type:      "sell",
			OrderType: "market",
			Volume:    decision.ProposedQuantity.String(),
			Pair:      pair.Symbol,
		},
		Holding: holding,
	}

	position.Publish()

	return position
}

/*
Publish the position to the UI, which will automatically marshal the Holding
and its Stoploss into the JSON payload. For clarity, the balance is kept out
of this, as there must be a way to get that more accurate to reality, where
the exchange publishes the wallet state at the sensible moments. The paper
trading implementation we use is based on the kraken-cli, where under normal
use you would also not be manually managing the balances.
*/
func (position *Position) Publish() {
	out := datura.NewMap()
	out["positions"] = []*Position{position}
	utils.Publish(position.ui, out)
}

/*
onTicker refreshes the mark cache for this position's holding and
lets the bound stoploss regulator evaluate the live bid path for
exit decisions.
*/
func (position *Position) onTicker(ticker kraken.TickerData) {
	position.mu.Lock()

	if position.Holding != nil && position.Holding.Stoploss != nil {
		if err := position.Holding.Update(ticker); err != nil {
			errnie.Error(err)
		}

		position.Holding.PnL = position.price.PnL(position.Holding)
		position.marked = true
	}

	position.mu.Unlock()
	position.Publish()
}

func (position *Position) onExecution(execution *kraken.Execution) {
	for _, row := range execution.Data {
		if row.Symbol != position.pair.Symbol || !position.matches(row) {
			continue
		}

		position.mu.Lock()
		feeAlreadyApplied := false

		if row.FeeUsdEquiv != nil && row.ExecID != "" {
			_, feeAlreadyApplied = position.executionFees[row.ExecID]

			if !feeAlreadyApplied {
				position.executionFees[row.ExecID] = struct{}{}
			}
		}

		if feeAlreadyApplied {
			row.FeeUsdEquiv = nil
		}

		if row.Side == "buy" {
			position.applyEntry(row)
		}

		if row.Side == "sell" {
			position.applyExit(row)
		}

		closed := position.Status == types.CLOSED
		position.mu.Unlock()
		position.Publish()

		if closed {
			position.cancel()
		}
	}
}

func (position *Position) matches(execution kraken.ExecutionData) bool {
	position.mu.RLock()
	defer position.mu.RUnlock()

	if execution.ClientOrderID != "" {
		if execution.ClientOrderID == position.ID {
			return true
		}

		if position.EntryOrder != nil && execution.ClientOrderID == position.EntryOrder.ClOrdId {
			return true
		}

		if position.ExitOrder != nil && execution.ClientOrderID == position.ExitOrder.ClOrdId {
			return true
		}
	}

	return execution.OrderID != "" &&
		(execution.OrderID == position.EntryOrderID || execution.OrderID == position.ExitOrderID)
}

func (position *Position) applyEntry(execution kraken.ExecutionData) {
	filled := execution.CumQty != nil && execution.CumQty.Sign() > 0

	if !filled {
		filled = execution.LastQty != nil && execution.LastQty.Sign() > 0
	}

	if !filled {
		return
	}

	if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
		position.Holding.Qty = execution.CumQty.Copy()
		position.Holding.SellableQty = execution.CumQty.Copy()
	}

	if execution.CumQty == nil && execution.LastQty != nil && execution.LastQty.Sign() > 0 {
		if position.Holding.SellableQty == nil {
			position.Holding.SellableQty = execution.LastQty.Copy()
		} else {
			position.Holding.SellableQty = position.Holding.SellableQty.Add(execution.LastQty)
		}

		position.Holding.Qty = position.Holding.SellableQty.Copy()
	}

	if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
		position.Holding.EntryPrice = execution.AvgPrice.Copy()
		mark := execution.AvgPrice

		if position.marked && position.Holding.Mark != nil {
			mark = position.Holding.Mark
		}

		position.Holding.Mark = mark.Copy()
		position.Holding.Stoploss.Bind(execution.AvgPrice, mark)

		if !position.entryFeeActual {
			feeRate, err := position.price.Fee(position.Holding.Symbol)

			if err != nil {
				errnie.Error(errnie.Err(
					errnie.NotFound,
					"position: could not estimate entry fee from fill",
					err,
				))
			}

			if err == nil {
				entryValue := decimal.ExactMul(execution.AvgPrice, position.Holding.Qty)
				position.Holding.EntryFee = decimal.ExactMul(entryValue, feeRate)
			}
		}
	}

	if execution.FeeUsdEquiv != nil {
		if !position.entryFeeActual || position.Holding.EntryFee == nil {
			position.Holding.EntryFee = execution.FeeUsdEquiv.Copy()
			position.entryFeeActual = true
		} else {
			feeScale := max(
				position.Holding.EntryFee.GetScale(),
				execution.FeeUsdEquiv.GetScale(),
			)
			position.Holding.EntryFee = position.Holding.EntryFee.
				SetScale(feeScale).
				Add(execution.FeeUsdEquiv)
		}
	}

	entryAt := execution.Timestamp

	if entryAt.IsZero() {
		entryAt = time.Now().UTC()
	}

	position.Holding.EntryAt = &entryAt
	position.Status = types.OPEN
	position.Holding.Status = types.OPEN
}

func (position *Position) applyExit(execution kraken.ExecutionData) {
	if execution.FeeUsdEquiv != nil {
		if position.Holding.ExitFee == nil {
			position.Holding.ExitFee = execution.FeeUsdEquiv.Copy()
		} else {
			feeScale := max(
				position.Holding.ExitFee.GetScale(),
				execution.FeeUsdEquiv.GetScale(),
			)
			position.Holding.ExitFee = position.Holding.ExitFee.
				SetScale(feeScale).
				Add(execution.FeeUsdEquiv)
		}
	}

	if execution.OrderStatus != "filled" {
		// An exit that will never fill still ends the lot.
		status, err := types.StatusFromMarket(execution.OrderStatus)

		if err == nil && isTerminal(status) {
			position.Status = status
			position.Holding.Status = status
			return
		}

		position.Status = types.PENDING
		position.Holding.Status = types.PENDING
		return
	}

	if execution.AvgPrice != nil && execution.AvgPrice.Sign() > 0 {
		position.Holding.ExitPrice = execution.AvgPrice.Copy()
	}

	exitAt := execution.Timestamp

	if exitAt.IsZero() {
		exitAt = time.Now().UTC()
	}

	position.Holding.ExitAt = &exitAt
	position.Holding.SellableQty = decimal.NewFromInt64(0)
	position.Status = types.CLOSED
	position.Holding.Status = types.CLOSED
}

/*
Enter submits a market buy for its quantity and returns the transport error so
Desk cannot publish a false entry-submitted lifecycle.
*/
func (position *Position) Enter() (*Position, error) {
	result, err := position.api.AddOrder(position.EntryOrder)

	if err != nil {
		position.mu.Lock()
		position.Status = types.ERROR
		position.Holding.Status = types.ERROR
		position.mu.Unlock()

		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.mu.Lock()

	if len(result.ID) > 0 {
		position.EntryOrderID = result.ID[0]
	}

	if position.Status != types.OPEN && position.Status != types.CLOSED {
		position.Status = types.PENDING
		position.Holding.Status = types.PENDING
	}
	position.mu.Unlock()
	position.Publish()

	return position, nil
}

/*
Exit submits a market sell for the sellable ledger quantity.
*/
func (position *Position) Exit(clientOrderID string) error {
	if clientOrderID == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position: exit order is missing a client order identifier",
			nil,
		))
	}

	position.mu.Lock()

	if position.Holding == nil || position.Holding.SellableQty == nil || position.Holding.SellableQty.Sign() <= 0 {
		position.mu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"position: no sellable quantity available",
			nil,
		))
	}

	position.ExitOrder.Volume = position.Holding.SellableQty.String()
	position.ExitOrder.ClOrdId = clientOrderID
	position.mu.Unlock()

	result, err := position.api.AddOrder(position.ExitOrder)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	position.mu.Lock()

	if len(result.ID) > 0 {
		position.ExitOrderID = result.ID[0]
	}

	if position.Status != types.CLOSED {
		position.Status = types.PENDING
		position.Holding.Status = types.PENDING
	}
	position.mu.Unlock()
	position.Publish()

	return nil
}

/*
Close marks the lot closed once Desk drops it from the open map.
*/
func (position *Position) Close() (err error) {
	position.mu.Lock()
	defer position.mu.Unlock()

	if position.Status == types.CLOSED {
		return nil
	}

	if position.cancel != nil {
		position.cancel()
	}

	if position.Holding != nil {
		err = errors.Join(err, position.Holding.Close())
	}

	position.Status = types.CLOSED
	return errnie.Error(err)
}
