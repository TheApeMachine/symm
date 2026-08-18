package broker

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
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
	ctx            context.Context
	cancel         context.CancelFunc
	api            *websocket.API
	ui             chan []byte
	instrument     *Instrument
	price          *Price
	balance        *Balance
	recorder       *audit.Recorder
	store          *PositionStore
	checkpoint     func()
	pair           kraken.InstrumentPair
	seenExecutions map[string]struct{}
	passage        *passageTracker
	Status         atomic.Pointer[types.Status] `json:"-"`
	/*
		Decision is the arbitration that opened this lot, kept verbatim.

		A position outlives the round that produced it: the planner moves on, and
		by the time anyone asks why a lot is open its originating decision is no
		longer anywhere in the current decision batch. Carrying it here is what
		lets the terminal answer that question at all, and it survives a client
		reconnect because the desk republishes its positions on connect.
	*/
	Decision         types.Decision        `json:"decision"`
	EntryOrder       *spot.AddOrderRequest `json:"entry_order"`
	ExitOrder        *spot.AddOrderRequest `json:"exit_order"`
	EntryOrderResult *spot.AddOrderResult  `json:"entry_order_result"`
	ExitOrderResult  *spot.AddOrderResult  `json:"exit_order_result"`
	Holding          *types.Holding        `json:"holding"`
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
	recorder *audit.Recorder,
	store *PositionStore,
	pair kraken.InstrumentPair,
	decision types.Decision,
) *Position {
	errnie.Info("creating position for: " + pair.Symbol)
	ctx, cancel := context.WithCancel(ctx)

	position := &Position{
		ctx:            ctx,
		cancel:         cancel,
		api:            api,
		ui:             ui,
		instrument:     instrument,
		price:          price,
		balance:        balance,
		recorder:       recorder,
		store:          store,
		pair:           pair,
		seenExecutions: map[string]struct{}{},
		Decision:       decision,
		EntryOrder: &spot.AddOrderRequest{
			ClOrdId:   decision.ID,
			Type:      "buy",
			OrderType: "market",
			Volume:    decision.ProposedQuantity.String(),
			Pair:      pair.Symbol,
		},
		Holding: &types.Holding{
			Symbol:        pair.Symbol,
			Qty:           decimal.NewFromInt64(0),
			SellableQty:   decimal.NewFromInt64(0),
			Asset:         pair.Base,
			EntryPrice:    decision.EntryPrice,
			Mark:          decision.Mark,
			IsOpportunity: decision.Opportunity,
			Stoploss:      decision.Stoploss,
		},
	}
	position.setStatus(types.INITIALIZING)

	return position
}

func (position *Position) status() types.Status {
	status := position.Status.Load()

	if status == nil {
		return types.UNKNOWN
	}

	return *status
}

func (position *Position) setStatus(status types.Status) {
	position.Status.Store(&status)
}

func (position *Position) MarshalJSON() ([]byte, error) {
	type positionJSON Position

	return json.Marshal(struct {
		Status types.Status `json:"status"`
		*positionJSON
	}{
		Status:       position.status(),
		positionJSON: (*positionJSON)(position),
	})
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
onTicker refreshes the mark cache for this position's holding and lets the
bound stoploss regulator judge the price a sale would actually realise.
*/
func (position *Position) onTicker(ticker kraken.TickerData) {
	if position.Holding == nil {
		position.Publish()
		return
	}

	position.price.Update(&ticker)

	if position.Holding.Qty == nil || position.Holding.Qty.Sign() <= 0 {
		if ticker.Bid != nil {
			position.Holding.Mark = ticker.Bid
		}

		position.Publish()
		return
	}

	if position.Holding.Stoploss == nil {
		position.Holding.Mark = ticker.Bid
		position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
		position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)
		position.Publish()
		return
	}

	previousStatus := position.Holding.Stoploss.Status
	previousLocked := position.Holding.Stoploss.Locked
	previousFloor := position.Holding.Stoploss.Floor
	previousPeak := position.Holding.Stoploss.Peak
	position.Holding.Update(ticker)
	position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
	position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)

	if position.passage != nil {
		position.passage.observe(position, position.Holding.Mark)
	}

	stoploss := position.Holding.Stoploss
	changed := previousStatus != stoploss.Status ||
		previousLocked != stoploss.Locked ||
		previousFloor.Cmp(stoploss.Floor) != 0 ||
		previousPeak.Cmp(stoploss.Peak) != 0

	if changed && position.store != nil {
		if err := position.store.Save(stoploss); err != nil {
			errnie.Error(errnie.Err(
				errnie.IO,
				"position: failed to persist stoploss transition",
				err,
			))
		}
	}

	if stoploss.Status == types.TRIGGERED && position.ExitOrder == nil {
		if position.checkpoint != nil {
			position.checkpoint()
		}

		if _, err := position.Exit(); err != nil {
			errnie.Error(err)
		}
	}

	position.Publish()
}

/*
MarkFeedback snapshots the live position geometry after one ticker has updated
the holding and its stop. Distances are dimensionless so the global regulator
can compare instruments without confusing quote-currency price scales.
*/
func (position *Position) MarkFeedback(at time.Time) types.MarkFeedback {
	feedback := types.MarkFeedback{}

	if position == nil || position.Holding == nil {
		return feedback
	}

	holding := position.Holding
	feedback.PositionID = position.Decision.ID
	feedback.Symbol = holding.Symbol
	feedback.At = at.UTC()

	if feedback.At.IsZero() {
		feedback.At = time.Now().UTC()
	}

	feedback.ReturnPct = holding.ReturnPct
	feedback.Exposed = holding.Qty != nil && holding.Qty.Sign() > 0

	if holding.Mark != nil {
		feedback.Mark = holding.Mark.Float64()
	}

	if holding.PnL != nil {
		feedback.PnL = holding.PnL.Float64()
	}

	stoploss := holding.Stoploss

	if stoploss == nil {
		return feedback
	}

	feedback.StopStatus = stoploss.Status
	feedback.TriggerReason = stoploss.TriggerReason
	feedback.SurgeArmed = stoploss.SurgeArmed

	if feedback.Mark > 0 && stoploss.Floor != nil {
		feedback.FloorDistance = (feedback.Mark - stoploss.Floor.Float64()) / feedback.Mark
	}

	if feedback.Mark > 0 && stoploss.Peak != nil && stoploss.Peak.Sign() > 0 {
		feedback.PeakDrawdown = math.Min(0, math.Log(feedback.Mark/stoploss.Peak.Float64()))
	}

	return feedback
}

func (position *Position) onExecution(message kraken.Execution) bool {
	for _, execution := range message.Data {
		if position.ExitOrder != nil &&
			execution.ClientOrderID == position.ExitOrder.ClOrdId {
			status, err := types.StatusFromMarket(execution.OrderStatus)

			if err != nil {
				position.setStatus(types.ERROR)
				position.Holding.Status = types.ERROR
				position.Publish()
				errnie.Error(err)
				return false
			}

			if status == types.FILLED {
				if err = position.closeFill(execution); err != nil {
					position.setStatus(types.ERROR)
					position.Holding.Status = types.ERROR
					position.Publish()
					errnie.Error(err)
					return false
				}

				return true
			}

			if status != types.CANCELED && status != types.REJECTED &&
				status != types.EXPIRED {
				continue
			}

			if execution.CumQty != nil && execution.CumQty.Sign() > 0 {
				position.setStatus(types.ERROR)
				position.Holding.Status = types.ERROR
				position.Publish()
				errnie.Error(errnie.Err(
					errnie.Conflict,
					"position: terminal exit retained partial inventory",
					nil,
				))
				return false
			}

			position.ExitOrder = nil
			position.ExitOrderResult = nil
			position.setStatus(types.OPEN)
			position.Holding.Status = types.OPEN
			position.Publish()
			continue
		}

		if position.EntryOrder == nil ||
			execution.ClientOrderID != position.EntryOrder.ClOrdId {
			continue
		}

		status, err := types.StatusFromMarket(execution.OrderStatus)

		if err != nil {
			position.setStatus(types.ERROR)
			position.Holding.Status = types.ERROR
			position.Publish()
			errnie.Error(err)
			return false
		}

		if status == types.CANCELED || status == types.REJECTED ||
			status == types.EXPIRED {
			position.setStatus(status)
			position.Holding.Status = status
			position.Publish()

			if position.cancel != nil {
				position.cancel()
			}

			return true
		}

		if execution.CumQty == nil || execution.CumQty.Sign() <= 0 ||
			execution.CumCost == nil || execution.CumCost.Sign() <= 0 ||
			execution.FeeUsdEquiv == nil || execution.FeeUsdEquiv.Sign() < 0 {
			continue
		}

		position.setStatus(status)
		position.Holding.Status = position.status()
		position.Holding.EntryAt = &execution.Timestamp
		position.Holding.EntryPrice = decimal.NewFromInt64(0).Add(
			execution.CumCost,
		).Div(execution.CumQty)
		position.Holding.EntryFee = execution.FeeUsdEquiv
		position.Holding.Qty = execution.CumQty
		position.Holding.SellableQty = execution.CumQty

		if err := position.Holding.Stoploss.RebindFill(
			position.Holding.EntryPrice,
			position.Holding.Mark,
		); err != nil {
			position.setStatus(types.ERROR)
			position.Holding.Status = types.ERROR
			position.Holding.Stoploss.Status = types.ERROR
			errnie.Error(err)
			continue
		}

		position.Holding.Stoploss.ArmClock()
		position.Holding.Stoploss.Update(position.Holding.Mark)
		position.passage = newPassageTracker(
			position,
			position.Holding.EntryPrice,
			max(1, position.Decision.ForecastHorizon),
		)
		position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
		position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)

		if position.store != nil {
			errnie.Error(position.store.Save(position.Holding.Stoploss))
		}
	}

	return false
}

/*
closeFill records the exchange's realized exit economics before the lot leaves
the desk and publishes the terminal state so retained UI positions can remove
it by identity.
*/
func (position *Position) closeFill(execution kraken.ExecutionData) error {
	if position.Holding == nil || position.Holding.Qty == nil ||
		position.Holding.Qty.Sign() <= 0 || position.Holding.EntryPrice == nil ||
		position.Holding.EntryPrice.Sign() <= 0 || position.Holding.EntryFee == nil ||
		position.Holding.EntryFee.Sign() < 0 || execution.CumQty == nil ||
		execution.CumQty.Sign() <= 0 || execution.CumCost == nil ||
		execution.CumCost.Sign() <= 0 || execution.FeeUsdEquiv == nil ||
		execution.FeeUsdEquiv.Sign() < 0 || execution.Timestamp.IsZero() {
		return errnie.Err(
			errnie.Validation,
			"position: complete exit fill economics required",
			nil,
		)
	}

	sellable := position.Holding.SellableQty

	if sellable == nil {
		sellable = position.Holding.Qty
	}

	if execution.CumQty.Cmp(sellable) != 0 {
		return errnie.Err(
			errnie.Conflict,
			"position: filled exit quantity does not match sellable inventory",
			nil,
		)
	}

	entryGross := decimal.NewFromInt64(0).Add(
		position.Holding.EntryPrice,
	).Mul(position.Holding.Qty)
	entryValue := entryGross.Add(position.Holding.EntryFee)
	exitValue := decimal.NewFromInt64(0).Add(execution.CumCost).Sub(
		execution.FeeUsdEquiv,
	)
	position.Holding.ExitAt = &execution.Timestamp
	position.Holding.ExitPrice = decimal.NewFromInt64(0).Add(
		execution.CumCost,
	).Div(execution.CumQty)
	position.Holding.ExitFee = execution.FeeUsdEquiv
	position.Holding.PnL = exitValue.Sub(entryValue)
	position.Holding.ReturnPct = decimal.NewFromInt64(0).Add(
		position.Holding.PnL,
	).Div(entryValue).Mul(decimal.NewFromInt64(100)).Float64()
	position.Holding.SellableQty = decimal.NewFromInt64(0)

	if position.store != nil {
		if err := position.store.Delete(position.pair.Symbol); err != nil {
			return err
		}
	}

	if err := position.Close(); err != nil {
		return err
	}

	position.Publish()

	return nil
}

/*
Enter submits a market buy for its quantity and returns the transport error so
Desk cannot publish a false entry-submitted lifecycle.
*/
func (position *Position) Enter() (*Position, error) {
	result, err := position.api.AddOrder(position.EntryOrder)

	if err != nil {
		position.setStatus(types.ERROR)
		position.Holding.Status = types.ERROR
		position.Holding.Stoploss.Status = types.ERROR

		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.EntryOrderResult = &result

	if position.status() != types.OPEN && position.status() != types.CLOSED {
		position.setStatus(types.PENDING)
		position.Holding.Status = types.PENDING
	}

	position.Publish()
	return position, nil
}

/*
Exit is the single sell-order boundary for an open lot. Exit causes may evolve,
but none may bypass the position's regulator and liquidate an armed holding.
*/
func (position *Position) Exit() (*Position, error) {
	if position.Holding == nil || position.Holding.Stoploss == nil ||
		position.Holding.Stoploss.Status != types.TRIGGERED {
		return position, errnie.Err(
			errnie.NotAcceptable,
			"position: triggered stoploss required to submit an exit",
			nil,
		)
	}

	exitOrder := &spot.AddOrderRequest{
		ClOrdId:   position.EntryOrder.ClOrdId + "-exit",
		Type:      "sell",
		OrderType: "market",
		Volume:    position.Holding.Qty.String(),
		Pair:      position.pair.Symbol,
	}

	result, err := position.api.AddOrder(exitOrder)

	if err != nil {
		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market exit order",
			err,
		))
	}

	position.ExitOrder = exitOrder
	position.ExitOrderResult = &result
	position.setStatus(types.PENDING)
	position.Holding.Status = types.PENDING

	position.Publish()
	return position, nil
}

/*
Close marks the lot closed once Desk drops it from the open map.
*/
func (position *Position) Close() (err error) {
	if position.status() == types.CLOSED {
		return nil
	}

	if position.cancel != nil {
		position.cancel()
	}

	if position.Holding != nil {
		err = errors.Join(err, position.Holding.Close())
	}

	position.setStatus(types.CLOSED)
	return errnie.Error(err)
}
