package broker

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

const (
	channelTicker     = "ticker"
	channelBalances   = "balances"
	channelExecutions = "executions"
	channelOrders     = "orders"
	channelAddOrder   = "add_order"
)

type Desk struct {
	ctx          context.Context
	cancel       context.CancelFunc
	channels     map[string]chan []byte
	public       websocket.Socket
	private      websocket.Private
	balance      *kraken.BalanceDataSlice
	positions    *sync.Map
	UIForward    chan []byte
	maxPositions int
	maxReserved  int
}

func NewDesk(
	ctx context.Context,
	public websocket.Socket,
	private websocket.Private,
	uiForward chan []byte,
) (*Desk, error) {
	ctx, cancel := context.WithCancel(ctx)

	if public == nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: public stream required",
			nil,
		))
	}

	if private == nil {
		cancel()
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: private stream required",
			nil,
		))
	}

	return &Desk{
		ctx:     ctx,
		cancel:  cancel,
		public:  public,
		private: private,
		channels: map[string]chan []byte{
			channelTicker:     public.Observe(channelTicker),
			channelBalances:   private.Observe(channelBalances),
			channelExecutions: private.Observe(channelExecutions),
			channelOrders:     private.Observe(channelOrders),
			channelAddOrder:   private.Observe(channelAddOrder),
		},
		positions:    &sync.Map{},
		UIForward:    uiForward,
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
	}, nil
}

func (desk *Desk) Ready() bool {
	return errnie.Require(map[string]any{
		"balance": desk.balance,
	}) == nil
}

func (desk *Desk) OpenPositions() int {
	count := 0

	desk.positions.Range(func(_, _ any) bool {
		count++
		return true
	})

	return count
}

/*
Run processes websocket and private frame streams until ctx closes.
*/
func (desk *Desk) Run() (err error) {
	for {
		select {
		case <-desk.ctx.Done():
			return nil
		case msg := <-desk.channels[channelBalances]:
			desk.balance = kraken.NewBalanceDataSlice(msg)
		case msg := <-desk.channels[channelExecutions]:
			for _, execution := range *kraken.NewExecutionDataSlice(msg) {
				position, ok := desk.positions.Load(execution.Symbol)

				if ok {
					position.(*Position).Execution(&execution)
				}
			}
		case msg := <-desk.channels[channelOrders]:
			for _, order := range *kraken.NewOrderDataSlice(msg) {
				symbol := order.Pair

				if symbol == "" {
					symbol = order.Description.Pair
				}

				position, ok := desk.positions.Load(symbol)

				if ok {
					position.(*Position).Order(&order)
				}
			}
		case msg := <-desk.channels[channelAddOrder]:
			order := kraken.NewOrderResponse(msg)

			desk.positions.Range(func(key any, value any) bool {
				position := value.(*Position)

				if position.orderID == order.Result.ClOrdID {
					position.OrderAck(order)
				}

				return true
			})
		case msg := <-desk.channels[channelTicker]:
			for _, ticker := range kraken.NewTickerDataSlice(msg) {
				position, ok := desk.positions.Load(ticker.Symbol)

				if ok {
					position.(*Position).AddTicker(&ticker)
				}
			}
		}

		out := datura.Map[any]{
			"positions":  make([]*PositionData, 0),
			"orders":     make([]*kraken.OrderData, 0),
			"executions": make([]*kraken.ExecutionData, 0),
		}

		desk.positions.Range(func(key any, value any) bool {
			position := value.(*Position)

			// While we are looping over the positions anyway, we can check
			// for positions that are ready to be removed, freeing up an empty
			// slot for a new trade to occupy.
			if slices.Contains(
				[]types.Status{types.CLOSED, types.FATAL}, position.status,
			) {
				desk.positions.Delete(key)
				return true
			}

			out["positions"] = append(out["positions"].([]*PositionData), position.data)

			if position.order != nil {
				out["orders"] = append(out["orders"].([]*kraken.OrderData), position.order)
			}

			if len(position.executions) > 0 {
				out["executions"] = append(
					out["executions"].([]*kraken.ExecutionData), position.executions...,
				)
			}

			return true
		})

		if desk.balance != nil {
			out["balance"] = desk.balance
		}

		desk.UIForward <- out.Marshal()
	}
}

func (desk *Desk) Holdings() map[string]PositionData {
	holdings := map[string]PositionData{}

	desk.positions.Range(func(key any, value any) bool {
		position := value.(*Position)
		holdings[key.(string)] = *position.data
		return true
	})

	return holdings
}

func (desk *Desk) Buy(
	symbol string,
	fraction float64,
	price decimal.Decimal,
	opportunity bool,
) error {
	if !desk.Ready() {
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"broker: desk not ready",
			nil,
		))
	}

	openPositions := desk.OpenPositions()

	if openPositions >= desk.maxPositions+desk.maxReserved {
		// We are not able to take on any new positions, not even a
		// high-value opportunity.
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"broker: max positions and max reserved reached",
			nil,
		))
	}

	if openPositions >= desk.maxPositions && !opportunity {
		// Given the proposed trade is not a high-value opportunity
		// we are unable to take on any new position at the moment.
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"broker: max positions reached",
			nil,
		))
	}

	if price.Rat().Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy price must be positive",
			nil,
		))
	}

	if fraction <= 0 || fraction > 1 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy fraction must be within the quote balance",
			nil,
		))
	}

	_, quote, ok := strings.Cut(strings.TrimSpace(symbol), "/")

	if !ok || strings.TrimSpace(quote) == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy symbol must include base and quote",
			nil,
		))
	}

	qty := 0.0

	for _, balance := range *desk.balance {
		if strings.EqualFold(balance.Asset, quote) {
			qty = balance.Available.Mul(
				decimal.NewFromFloat64(fraction),
			).Div(&price).Float64()
			break
		}
	}

	if qty <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy quantity must be positive",
			nil,
		))
	}

	position, ok := desk.positions.LoadOrStore(symbol, NewPosition(
		desk.private,
		&PositionData{
			Symbol:     symbol,
			Qty:        qty,
			EntryPrice: price,
		},
	))

	if ok {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"symbol already has open position",
			nil,
		))
	}

	return position.(*Position).Enter()
}

func (desk *Desk) Sell(symbol string) (err error) {
	if !desk.Ready() {
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"broker: desk not ready",
			nil,
		))
	}

	position, ok := desk.positions.Load(symbol)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"position not found for symbol",
			nil,
		))
	}

	return position.(*Position).Exit()
}

func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}
