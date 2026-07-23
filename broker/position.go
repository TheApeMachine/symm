package broker

import (
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Position struct {
	status     types.Status
	api        *websocket.API
	instrument *Instrument
	price      *Price
	balance    *Balance
	pair       *kraken.InstrumentPair
	request    *kraken.MarketOrder
	order      *spot.Order
	orderID    string
}

/*
NewPosition constructs one lot shell; Desk routes order and execution frames.
*/
func NewPosition(
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair *kraken.InstrumentPair,
) *Position {
	return &Position{
		status:     types.INITIALIZING,
		api:        api,
		instrument: instrument,
		price:      price,
		balance:    balance,
		pair:       pair,
		order: &spot.Order{
			Description: &spot.OrderDescription{
				Pair:      pair.Symbol,
				Type:      "enter",
				OrderType: "market",
			},
			Volume: decimal.NewFromFloat64(0),
			Price:  decimal.NewFromFloat64(0),
		},
	}
}

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

	position.status = types.CLOSED
}

/*
Mark applies the latest mid for this lot and feeds its Stoploss.
*/
func (position *Position) Mark(symbol string) {
	if symbol != position.pair.Symbol {
		return
	}

	value, ok := position.balance.holdings.Load(position.pair.Symbol)

	if !ok {
		return
	}

	holding := value.(*types.Holding)

	if holding.Status != types.OPEN {
		return
	}

	_ = position.price.Mark(position.pair, holding)

	if holding.Stoploss != nil && holding.StopMark != nil {
		holding.Stoploss.ObserveMark(holding.StopMark.Float64())
	}
}

func (position *Position) OrderAck(buf []byte) {
	orderAck := kraken.NewOrderResponse(buf)

	if position.request == nil || orderAck.ReqID != position.request.ReqID {
		return
	}

	if errnie.Error(kraken.Validate(orderAck)) != nil {
		position.request = nil
		position.status = types.ERROR
		return
	}

	position.orderID = orderAck.Result.OrderID
	position.status = types.PENDING
}

func (position *Position) ExecutionAck(buf []byte) {
	execution := kraken.NewExecution(buf)

	if errnie.Error(kraken.Validate(execution)) != nil {
		return
	}

	for _, data := range execution.Data {
		if data.OrderID != position.orderID {
			continue
		}

		holding := position.holding(data.Symbol)

		if holding == nil {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"holding not found for "+data.Symbol,
				nil,
			))

			continue
		}

		data.Side = strings.ToLower(data.Side)
		position.price.Fill(position.pair, holding, data)

		status, ok := types.MarketStatuses[data.ExecType]

		if !ok {
			status = types.Status(data.ExecType)
		}

		if holding.Qty == nil || holding.Qty.Sign() <= 0 {
			status = types.CLOSED
		}

		holding.Status = status
		position.status = status
		position.balance.Publish()

		if holding.Status == types.CLOSED || holding.Status == types.CANCELED {
			position.Close()
		}
	}
}

/*
holding resolves the lot for an execution symbol. Compact paper/history pairs
(NEARUSD) are canonicalized through the SDK normalizer on the API.
*/
func (position *Position) holding(symbol string) *types.Holding {
	if value, ok := position.balance.holdings.Load(position.pair.Symbol); ok {
		return value.(*types.Holding)
	}

	for _, key := range []string{symbol, position.api.Name(symbol)} {
		if key == "" {
			continue
		}

		if value, ok := position.balance.holdings.Load(key); ok {
			return value.(*types.Holding)
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

	position.balance.holdings.Store(holding.Symbol, holding)

	amount, err := position.price.Taker(position.pair, holding.Qty)

	if err != nil {
		position.balance.holdings.Delete(holding.Symbol)

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate taker cost: "+err.Error(),
			err,
		))
	}

	if !position.balance.Available(amount) {
		position.balance.holdings.Delete(holding.Symbol)

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"insufficient balance",
			nil,
		))
	}

	position.request = kraken.NewMarketOrder(
		"buy",
		holding.Qty,
		holding.Symbol,
	)

	position.status = types.PENDING

	if err := position.api.AddOrder(position.request); err != nil {
		position.balance.holdings.Delete(holding.Symbol)
		position.request = nil
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
Exit submits a market sell for the full filled quantity.
*/
func (position *Position) Exit() error {
	if position.status == types.PENDING {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"position is pending",
			nil,
		))
	}

	holding, err := position.balance.Holding(position.pair.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"failed to get holding for "+position.pair.Symbol,
			err,
		))
	}

	if holding.Qty == nil || holding.Qty.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"quantity must be positive for "+position.pair.Symbol,
			nil,
		))
	}

	asset := holding.Asset

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

	position.request = kraken.NewMarketOrder(
		"sell",
		holding.Qty,
		holding.Symbol,
	)

	prior := position.status
	position.status = types.PENDING

	if err := position.api.AddOrder(position.request); err != nil {
		position.request = nil
		position.status = prior

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market sell order",
			err,
		))
	}

	return nil
}
