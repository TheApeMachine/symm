package broker

import (
	"context"
	"errors"
	"sync/atomic"

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
	ctx            context.Context
	cancel         context.CancelFunc
	api            *websocket.API
	ui             chan []byte
	instrument     *Instrument
	price          *Price
	balance        *Balance
	pair           kraken.InstrumentPair
	seenExecutions map[string]struct{}
	entryFills     int
	exitFills      int
	/*
		evidence is the latest reading the strategy has published about this
		symbol. It is an atomic handoff because the strategy runs on the
		analyzer's goroutine while everything else here runs on the desk's, and
		the alternative — letting the strategy reach in and set stop geometry
		directly — puts two goroutines on the same regulator.

		The desk-side reader takes whatever is currently latched. A tick that
		arrives between two thesis cuts uses the previous cut's reading, which
		is the correct behaviour: toxicity does not stop being true because no
		new measurement landed this millisecond.
	*/
	evidence         atomic.Pointer[types.StopEvidence]
	ID               string                `json:"id"`
	Status           types.Status          `json:"status"`
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
	pair kraken.InstrumentPair,
	decision types.Decision,
) *Position {
	errnie.Info("creating position for: " + pair.Symbol)
	ctx, cancel := context.WithCancel(ctx)

	holding := types.NewHolding(
		ctx,
		pair.Symbol,
		decision,
	)

	position := &Position{
		ctx:            ctx,
		cancel:         cancel,
		Status:         types.INITIALIZING,
		api:            api,
		ui:             ui,
		instrument:     instrument,
		price:          price,
		balance:        balance,
		pair:           pair,
		seenExecutions: map[string]struct{}{},
		ID:             decision.ID,
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
ApplyEvidence latches the strategy's latest reading of this symbol.

It only stores. Nothing is evaluated on the caller's goroutine, so a thesis cut
can never race the desk into the regulator, and the reading is applied on the
next tick alongside the book it belongs with.
*/
func (position *Position) ApplyEvidence(evidence types.StopEvidence) {
	if position == nil || !evidence.Present {
		return
	}

	position.evidence.Store(&evidence)
}

/*
observation assembles what the regulator judges: the price this lot's own size
could actually be sold at, plus whatever the strategy last said about whether
that price is real.
*/
func (position *Position) observation() types.StopEvidence {
	evidence := types.StopEvidence{Symbol: position.pair.Symbol}

	if latched := position.evidence.Load(); latched != nil {
		evidence = *latched
	}

	quantity := position.Holding.SellableQty

	if quantity == nil || quantity.Sign() <= 0 {
		quantity = position.Holding.Qty
	}

	evidence.ExecutableMark = position.price.ExecutableMark(position.pair, quantity)
	evidence.Present = evidence.ExecutableMark != nil

	if tick := position.price.Tick(position.pair.Symbol); tick != nil {
		evidence.SellCapacity = decimal.NewFromFloat64(tick.BidQty)
	}

	/*
		Spread and impact are left exactly as the strategy stated them. They are
		a matched pair — the impact estimate is a fraction of the spread it was
		measured against — and overwriting one of them here with a fresher tick
		would leave the regulator re-deriving its noise band from two readings
		of two different moments.
	*/
	return evidence
}

/*
onTicker refreshes the mark cache for this position's holding and lets the
bound stoploss regulator judge the price a sale would actually realise.
*/
func (position *Position) onTicker(ticker kraken.TickerData) {
	if position.Holding != nil {
		if err := position.Holding.Update(ticker); err != nil {
			errnie.Error(err)
		}

		// A lot adopted from the wallet was constructed before anything had
		// priced its symbol, so this is the first moment a boundary can be
		// drawn for it. Adopt is a no-op once the regulator has geometry.
		if position.Holding.Stoploss != nil {
			position.Holding.Stoploss.Adopt(position.price.RiskPlan(position.pair))
		}

		position.Holding.Observe(position.observation())
		position.Holding.PnL = position.price.PnL(position.pair, position.Holding)
		position.Holding.ReturnPct = position.price.ReturnPct(position.pair, position.Holding)
	}

	if position.Status == types.OPEN && position.Holding.Stoploss != nil &&
		position.Holding.Stoploss.Status == types.TRIGGERED {
		position.Exit(position.ExitOrder.ClOrdId)
	}

	position.Publish()
}

func (position *Position) onExecution(execution *kraken.Execution) {
	for _, row := range execution.Data {
		clientOrderID := position.EntryOrder.ClOrdId

		if row.Side == "sell" {
			clientOrderID = position.ExitOrder.ClOrdId
		}

		if row.Symbol != position.pair.Symbol || row.ClientOrderID != clientOrderID {
			continue
		}

		if row.ExecType != "trade" || row.CumQty == nil || row.CumQty.Sign() <= 0 {
			continue
		}

		if row.ExecID != "" {
			if _, seen := position.seenExecutions[row.ExecID]; seen {
				continue
			}

			position.seenExecutions[row.ExecID] = struct{}{}
		}

		if row.Side == "buy" {
			position.Holding.EntryPrice = row.AvgPrice

			if row.FeeUsdEquiv != nil && position.entryFills == 0 {
				position.Holding.EntryFee = row.FeeUsdEquiv.Copy()
			}

			if row.FeeUsdEquiv != nil && position.entryFills > 0 {
				position.Holding.EntryFee = position.Holding.EntryFee.Add(row.FeeUsdEquiv)
			}

			// We set it now and update it on every ticker update.
			position.Holding.ExitPrice = row.AvgPrice
			position.Holding.Mark = row.AvgPrice

			position.Holding.Qty = row.CumQty
			position.Holding.SellableQty = row.CumQty
			position.entryFills++

			/*
				The regulator was armed against the ask the order was priced at.
				The venue has now said what was actually paid, on what quantity,
				at what fee — so the break-even and profit lines are re-solved
				from the realized basis rather than left defending an estimate
				that is wrong in the direction which makes the lot look more
				profitable than it is.

				Rebinding on every fill is deliberate: the fields above are
				cumulative, so a partially filled entry that fills the rest a
				moment later moves the true basis, and the last call wins.
			*/
			position.Holding.Stoploss.RebindFill(types.Fill{
				EntryPrice: position.Holding.EntryPrice,
				EntryFee:   position.Holding.EntryFee,
				Qty:        position.Holding.Qty,
			})

			position.Status = types.OPEN
			position.Holding.Status = types.OPEN
			position.Holding.Stoploss.Status = types.ARMED
		}

		if row.Side == "sell" {
			position.Holding.ExitPrice = row.AvgPrice

			if row.FeeUsdEquiv != nil && position.exitFills == 0 {
				position.Holding.ExitFee = row.FeeUsdEquiv.Copy()
			}

			if row.FeeUsdEquiv != nil && position.exitFills > 0 {
				position.Holding.ExitFee = position.Holding.ExitFee.Add(row.FeeUsdEquiv)
			}

			position.exitFills++

			if row.OrderStatus != "filled" {
				position.Publish()
				continue
			}

			position.Holding.SellableQty = decimal.NewFromInt64(0)

			position.Status = types.CLOSED
			position.Holding.Status = types.CLOSED
		}

		position.Publish()
	}
}

/*
Enter submits a market buy for its quantity and returns the transport error so
Desk cannot publish a false entry-submitted lifecycle.
*/
func (position *Position) Enter() (*Position, error) {
	result, err := position.api.AddOrder(position.EntryOrder)

	if err != nil {
		position.Status = types.ERROR
		position.Holding.Status = types.ERROR

		return position, errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	position.EntryOrderResult = &result

	if position.Status != types.OPEN && position.Status != types.CLOSED {
		position.Status = types.PENDING
		position.Holding.Status = types.PENDING
	}

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

	if position.Holding == nil || position.Holding.SellableQty == nil || position.Holding.SellableQty.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"position: no sellable quantity available",
			nil,
		))
	}

	position.ExitOrder.Volume = position.Holding.SellableQty.String()
	position.ExitOrder.ClOrdId = clientOrderID

	result, err := position.api.AddOrder(position.ExitOrder)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	position.ExitOrderResult = &result

	if position.Status != types.CLOSED {
		position.Status = types.PENDING
		position.Holding.Status = types.PENDING
	}

	position.Publish()
	return nil
}

/*
Close marks the lot closed once Desk drops it from the open map.
*/
func (position *Position) Close() (err error) {
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
