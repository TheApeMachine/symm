package broker

import (
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
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
	status     types.Status
	api        *websocket.API
	instrument *Instrument
	price      *Price
	balance    *Balance
	pair       kraken.InstrumentPair
	request    *kraken.MarketOrder
	order      *spot.Order
	orderID    string
	intentID   string
	fills      []Fill
	seenExec   map[string]struct{}
	buffered   []kraken.ExecutionData
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
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	pair kraken.InstrumentPair,
) *Position {
	return &Position{
		status:     types.INITIALIZING,
		api:        api,
		instrument: instrument,
		price:      price,
		balance:    balance,
		pair:       pair,
		seenExec:   make(map[string]struct{}),
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

	next, err := types.Transition(position.status, types.CLOSED)

	if err != nil {
		errnie.Error(err)

		return
	}

	position.status = next
}

/*
setStatus applies a canonical Transition and fails loud on illegal edges.
*/
func (position *Position) setStatus(next types.Status) error {
	status, err := types.Transition(position.status, next)

	if err != nil {
		return err
	}

	position.status = status

	return nil
}

/*
Mark applies the latest bid (PnL) and StopMark (mid/last) for this lot and
feeds its Stoploss from StopMark — never from the flatten bid.
*/
func (position *Position) Mark(symbol string) {
	if symbol != position.pair.Symbol {
		return
	}

	_ = position.balance.Update(position.pair.Symbol, func(holding *types.Holding) error {
		if holding.Status != types.OPEN {
			return nil
		}

		_ = position.price.Mark(&position.pair, holding)

		if holding.Stoploss != nil && holding.StopMark != nil {
			holding.Stoploss.ObserveMark(holding.StopMark.Float64())
		}

		return nil
	})
}

func (position *Position) Symbol() string {
	return position.pair.Symbol
}
