package broker

import (
	"context"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Position is one lot shell owned by Desk. Order correlation uses request ID then
exchange order ID; unmatched executions buffer until the ack binds them.
*/
type Position struct {
	*types.Actor
	ctx        context.Context
	cancel     context.CancelFunc
	status     types.Status
	api        *websocket.API
	instrument *Instrument
	price      *Price
	balance    *Balance
	pair       kraken.InstrumentPair
	entryOrder *kraken.MarketOrder
	exitOrder  *kraken.MarketOrder
	orderID    string
	intentID   string
	fills      []Fill
	seenExec   map[string]struct{}
	buffered   []kraken.ExecutionData
	holding    *types.Holding
}

/*
Fill is one immutable execution print used to derive lot economics.
*/
type Fill struct {
	ExecID string
	Side   string
	Qty    *decimal.Decimal
	Price  *decimal.Decimal
	Fee    *decimal.Decimal
}

/*
NewPosition constructs one lot shell; Desk routes order and execution rows.
*/
func NewPosition(
	ctx context.Context,
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair kraken.InstrumentPair,
	qty *decimal.Decimal,
) *Position {
	ctx, cancel := context.WithCancel(ctx)

	entryOrder := kraken.NewMarketOrder(
		"buy", pair.Symbol, qty,
	)

	exitOrder := kraken.NewMarketOrder(
		"sell", pair.Symbol, qty,
	)

	position := &Position{
		ctx:        ctx,
		cancel:     cancel,
		status:     types.INITIALIZING,
		api:        api,
		instrument: instrument,
		price:      price,
		balance:    balance,
		pair:       pair,
		seenExec:   make(map[string]struct{}),
		entryOrder: entryOrder,
		exitOrder:  exitOrder,
	}

	position.holding = types.NewHolding(
		ctx,
		pair.Symbol,
		entryOrder.Params.OrderQty,
		entryOrder.Params.LimitPrice,
		position.Exit,
	)

	position.Actor = types.NewActor(ctx, map[string]types.Handler{
		"add_order":  {Topic: "add_order", Fn: position.onOrder},
		"executions": {Topic: "executions", Fn: position.onExecutions},
	})

	return position
}

/*
Status reports the lot lifecycle.
*/
func (position *Position) Status() types.Status {
	return position.status
}

/*
Close marks the lot closed once Desk drops it from the open map.
*/
func (position *Position) Close() {
	if position.status == types.CLOSED {
		return
	}

	position.holding.Close()
	position.status = types.CLOSED
}

func (position *Position) onOrder(message any) any {
	row := message.(*kraken.OrderResponse).Result
	position.orderID = row.OrderID
	position.status = types.PENDING
	return nil
}

func (position *Position) onExecutions(message any) any {
	rows := message.(*kraken.Execution).Data

	for _, row := range rows {
		symbol := position.holding.Symbol

		if symbol == "" {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"position: nil holding for execution symbol "+row.Symbol,
				nil,
			))

			return nil
		}

		if row.ExecID != "" {
			position.seenExec[row.ExecID] = struct{}{}
		}

		row.Side = strings.ToLower(row.Side)
		closed := false

		if err := position.balance.Update(symbol, func(holding *types.Holding) error {
			_ = position.price.RecordFill(&position.pair, holding, row, &position.fills)

			status, err := types.StatusFromMarket(row.ExecType)

			if err != nil {
				return err
			}

			if holding.Qty == nil || holding.Qty.Sign() <= 0 {
				status = types.CLOSED
			}

			if row.Side == "sell" && row.OrderStatus == "filled" {
				status = types.CLOSED
			}

			position.status = status
			position.holding.Status = status
			return nil
		}); err != nil {
			errnie.Error(err)
			return nil
		}

		if position.intentID != "" && position.balance != nil {
			_ = position.balance.Commit(position.intentID)
			position.intentID = ""
		}

		if err := position.balance.Publish(); err != nil {
			errnie.Error(err)
		}

		if closed {
			position.Close()
		}
	}

	return nil
}

/*
Enter seeds the holding onto Balance and submits a market buy for its quantity.
*/
func (position *Position) Enter(holding *types.Holding) error {
	if holding.Asset == "" {
		holding.Asset = position.pair.Base
	}

	position.balance.StoreHolding(holding)

	amount, err := position.price.Taker(&position.pair, holding.Qty)

	if err != nil {
		position.balance.DeleteHolding(holding.Symbol)

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate taker cost: "+err.Error(),
			err,
		))
	}

	// Desk already reserved cash under intentID; Available would subtract it twice.
	if position.intentID == "" {
		ok, err := position.balance.Available(amount)

		if err != nil {
			position.balance.DeleteHolding(holding.Symbol)

			return errnie.Error(err)
		}

		if !ok {
			position.balance.DeleteHolding(holding.Symbol)

			return errnie.Error(errnie.Err(
				errnie.Internal,
				"insufficient balance",
				nil,
			))
		}
	}

	if err := position.api.AddOrder(position.entryOrder); err != nil {
		position.balance.DeleteHolding(holding.Symbol)
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

/*
Exit submits a market sell for the sellable ledger quantity.
*/
func (position *Position) Exit() error {
	if position.status == types.PENDING {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"position is pending",
			nil,
		))
	}

	asset := position.holding.Asset

	if asset == "" {
		asset = position.pair.Base
	}

	available, err := position.balance.AssetAvailable(asset)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"no wallet availability to sell "+position.pair.Symbol,
			err,
		))
	}

	if available.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"no sellable "+asset+" available for "+position.pair.Symbol,
			nil,
		))
	}

	quantity := position.price.Quantize(&position.pair, available)

	if quantity == nil || quantity.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"exit quantity not executable for "+position.pair.Symbol,
			nil,
		))
	}

	if position.holding.Qty != nil && quantity.Cmp(position.holding.Qty) > 0 {
		quantity = position.price.Quantize(&position.pair, position.holding.Qty)
	}

	intentID := "exit:" + position.pair.Symbol + ":" + quantity.String()

	if err := position.balance.ReserveAsset(
		intentID, asset, quantity,
	); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"failed to reserve sellable "+asset+" for "+position.pair.Symbol,
			err,
		))
	}

	position.intentID = intentID
	prior := position.status

	if err := position.api.AddOrder(position.exitOrder); err != nil {
		errnie.Error(position.balance.Release(intentID))
		position.intentID = ""
		position.exitOrder = nil
		position.status = prior

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	return nil
}
