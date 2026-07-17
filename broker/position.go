package broker

import (
	"context"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/strategy"
	"github.com/theapemachine/symm/types"
)

type StopData struct {
	Symbol     string          `json:"symbol"`
	Armed      bool            `json:"-"`
	PeakPrice  decimal.Decimal `json:"-"`
	StopPrice  decimal.Decimal `json:"stop_price"`
	PeakReturn float64         `json:"peak_return"`
	StopReturn float64         `json:"stop_return"`
}

type Position struct {
	status     types.Status
	api        *websocket.API
	instrument *Instrument
	price      *Price
	balance    *Balance
	pair       *kraken.InstrumentPair
	request    *kraken.MarketOrder
	orderID    string
	Stop       *StopData
	stoploss   *strategy.Stoploss
	tickers    []*kraken.TickerData
	seenExec   sync.Map
	onTerminal func(symbol string)
}

/*
NewPosition constructs a position manager. Callbacks are registered once on
Desk so closed positions do not accumulate forever on the hot ticker path.
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
		tickers:    make([]*kraken.TickerData, 0),
		Stop:       &StopData{Symbol: pair.Symbol},
		stoploss:   strategy.NewStoploss(context.Background()),
	}
}

func (position *Position) Status() types.Status {
	return position.status
}

func (position *Position) OrderAck(buf []byte) {
	orderAck := kraken.NewOrderResponse(buf)

	if position.request == nil || orderAck.ReqID != position.request.ReqID {
		return
	}

	if errnie.Error(kraken.Validate(orderAck)) != nil {
		position.status = types.ERROR
		position.notifyTerminal()
		return
	}

	position.orderID = orderAck.Result.OrderID
	position.status = types.PENDING
}

func (position *Position) ExecutionAck(buf []byte) {
	execution := kraken.NewExecution(buf)

	if errnie.Error(kraken.Validate(execution)) != nil {
		position.status = types.ERROR
		position.notifyTerminal()
		return
	}

	for _, data := range execution.Data {
		if data.OrderID != position.orderID {
			continue
		}

		if data.ExecID != "" {
			if _, seen := position.seenExec.LoadOrStore(data.ExecID, struct{}{}); seen {
				continue
			}
		}

		value, ok := position.balance.holdings.Load(data.Symbol)

		if !ok {
			errnie.Error(errnie.Err(
				errnie.Internal,
				"holding not found for "+data.Symbol,
				nil,
			))

			return
		}

		holding := value.(*types.Holding)
		holding.Update(&data)
		holding.Mark = &data.LastPrice
		holding.Executions = append(holding.Executions, execution)
		position.status = holding.Status

		if holding.Closed() || holding.Status == types.CLOSED {
			position.status = types.CLOSED
			position.notifyTerminal()
		}

		return
	}
}

func (position *Position) Enter() error {
	holding, err := position.balance.Holding(position.pair.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get holding for "+position.pair.Symbol,
			err,
		))
	}

	qty := position.pair.RoundQty(holding.Qty)

	if qty == nil || qty.Cmp(decimal.NewFromFloat64(position.pair.QtyMin)) < 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position quantity is less than minimum quantity",
			nil,
		))
	}

	amount, err := position.price.Taker(position.pair, qty)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to calculate taker cost: "+err.Error(),
			err,
		))
	}

	if !position.pair.MeetsCostMin(amount) {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position notional is below instrument cost_min",
			nil,
		))
	}

	if !position.balance.Available(*amount) {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"insufficient balance",
			nil,
		))
	}

	holding.Qty = qty
	position.balance.holdings.Store(holding.Symbol, &holding)

	request := kraken.NewMarketOrder(
		"buy",
		qty.Float64(),
		holding.Symbol,
	)

	position.request = request
	position.status = types.PENDING

	if err := position.api.AddOrder(position.request); err != nil {
		position.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

func (position *Position) Exit() error {
	holding, err := position.balance.Holding(position.Stop.Symbol)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get holding for "+position.Stop.Symbol,
			err,
		))
	}

	qty := position.pair.RoundQty(holding.Qty)

	if qty == nil || qty.Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"position has no filled quantity to exit",
			nil,
		))
	}

	request := kraken.NewMarketOrder(
		"sell",
		qty.Float64(),
		holding.Symbol,
	)

	position.request = request
	position.status = types.PENDING

	if err := position.api.AddOrder(position.request); err != nil {
		position.status = types.OPEN

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to place market order",
			err,
		))
	}

	return nil
}

/*
TickerAck updates this position from its own ticker only.

Ticker updates only the current mark. Execution data remains authoritative for
entry and exit facts, and no mark-to-market value is invented here.
*/
func (position *Position) TickerAck(buf []byte) {
	value, ok := position.balance.holdings.Load(position.Stop.Symbol)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"holding not found for "+position.Stop.Symbol,
			nil,
		))

		return
	}

	ticker := kraken.NewTicker(buf)

	if errnie.Error(kraken.Validate(ticker)) != nil {
		return
	}

	for _, tickerData := range ticker.Data {
		if tickerData.Symbol != position.Stop.Symbol {
			continue
		}

		value.(*types.Holding).Mark = tickerData.Last
		return
	}
}

/*
Regulate updates the composed Stoploss from logic Evidence and mirrors the
numeric surface onto StopData for UI / audit without inventing mark prices.
*/
func (position *Position) Regulate(evidence strategy.Evidence) strategy.Verdict {
	verdict := position.stoploss.Update(evidence)
	armed, entry, peakReturn, stopReturn, _, _, _ := position.stoploss.State()
	position.Stop.Armed = armed
	position.Stop.PeakReturn = peakReturn
	position.Stop.StopReturn = stopReturn

	if armed && entry > 0 {
		position.Stop.PeakPrice = *decimal.NewFromFloat64(entry * (1 + peakReturn))
		position.Stop.StopPrice = *decimal.NewFromFloat64(entry * (1 + stopReturn))
	}

	return verdict
}

func (position *Position) notifyTerminal() {
	if position.onTerminal == nil || position.pair == nil {
		return
	}

	position.onTerminal(position.pair.Symbol)
}
