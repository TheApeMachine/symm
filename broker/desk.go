package broker

import (
	"context"
	"math"
	"math/big"
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
	ctx             context.Context
	cancel          context.CancelFunc
	status          types.Status
	channels        map[string]chan []byte
	public          websocket.PublicSocket
	private         websocket.Private
	balance         *kraken.BalanceDataSlice
	positions       *sync.Map
	UIForward       chan []byte
	maxPositions    int
	maxReserved     int
	feeSchedule     *sync.Map
	fallbackFeeRate float64
}

func NewDesk(
	ctx context.Context,
	public websocket.PublicSocket,
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
		status:  types.INITIALIZING,
		public:  public,
		private: private,
		channels: map[string]chan []byte{
			channelTicker:     public.Observe(channelTicker),
			channelBalances:   private.Observe(channelBalances),
			channelExecutions: private.Observe(channelExecutions),
			channelOrders:     private.Observe(channelOrders),
			channelAddOrder:   private.Observe(channelAddOrder),
		},
		positions:       &sync.Map{},
		UIForward:       uiForward,
		maxPositions:    viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:     viper.GetViper().GetInt("trading.slots.reserved"),
		feeSchedule:     &sync.Map{},
		fallbackFeeRate: viper.GetViper().GetFloat64("trading.paper.taker_fee_bps") / 10000,
	}, nil
}

func (desk *Desk) Status() types.Status {
	return desk.status
}

/*
Ready reports whether the account has hydrated — a balance snapshot has landed —
which is the precondition for sizing a new Buy. It is deliberately orthogonal to
the capacity status and is never consulted for a Sell: a close must always be
allowed through, hydrated or not.
*/
func (desk *Desk) Ready() bool {
	return desk.balance != nil
}

/*
refreshStatus derives the desk status from the live open-position count. It is the
single source of truth for the capacity state and must run whenever the open count
can change (a fill lands, a closed position is reaped) so the desk never stays
wedged in a full state after a slot frees up.

  - READY:    a normal slot is free — accepts both Buy and Sell.
  - PRIORITY: normal slots full, reserved slots remain — accepts only reserved
    (opportunity) Buys; Sell still flows.
  - BUSY:     normal and reserved slots both full — no Buys; Sell still flows.

Sell is never gated on status: a close must always be allowed through, in every
state, so a full book can always reclaim a slot by exiting.
*/
func (desk *Desk) refreshStatus() {
	open := desk.OpenPositions()

	switch {
	case open >= desk.maxPositions+desk.maxReserved:
		desk.status = types.BUSY
	case open >= desk.maxPositions:
		desk.status = types.PRIORITY
	default:
		desk.status = types.READY
	}
}

func (desk *Desk) OpenPositions() int {
	count := 0

	desk.positions.Range(func(_, _ any) bool {
		count++
		return true
	})

	return count
}

func (desk *Desk) Positions() []*Position {
	positions := make([]*Position, 0, desk.OpenPositions())

	desk.positions.Range(func(_, value any) bool {
		positions = append(positions, value.(*Position))
		return true
	})

	return positions
}

// SetFeeSchedule installs the per-symbol taker fee schedule and pushes each
// open position its own symbol's taker rate (falling back to the account tier
// for symbols Kraken did not itemize).
func (desk *Desk) SetFeeSchedule(schedule websocket.FeeSchedule) error {
	for key, value := range schedule.Pairs {
		desk.feeSchedule.Store(key, value)
	}

	if schedule.Fallback.Taker > 0 {
		desk.fallbackFeeRate = schedule.Fallback.Taker
	}

	return nil
}

// takerRate returns the taker fee fraction for symbol from the installed
// schedule.
func (desk *Desk) takerRate(symbol string) float64 {
	schedule, ok := desk.feeSchedule.Load(symbol)

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound, "schedule not found for "+symbol, nil,
		))

		return desk.fallbackFeeRate
	}

	switch val := schedule.(type) {
	case websocket.FeeRates:
		return val.Taker
	case map[string]websocket.FeeRates:
		return val["fee"].Taker
	default:
		return desk.fallbackFeeRate
	}
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
			desk.Executions(kraken.NewExecutionDataSlice(msg))
		case msg := <-desk.channels[channelOrders]:
			for _, order := range *kraken.NewOrderDataSlice(msg) {
				symbol := strings.TrimSpace(order.Pair)

				if symbol == "" {
					symbol = strings.TrimSpace(order.Description.Pair)
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

		// The open count may have dropped from reaping a closed position above,
		// so recompute the capacity status every loop. This is what unwedges the
		// desk from PRIORITY/BUSY once a slot frees up.
		desk.refreshStatus()

		if desk.balance != nil {
			out["balance"] = desk.balance
		}

		desk.UIForward <- out.Marshal()
	}
}

/*
Executions reconciles the exchange-owned open-position view.
*/
func (desk *Desk) Executions(executions *kraken.ExecutionDataSlice) {
	snapshot := len(*executions) == 0
	snapshotSymbols := map[string]struct{}{}

	for _, execution := range *executions {
		symbol := strings.TrimSpace(execution.Symbol)

		if symbol == "" {
			errnie.Error(errnie.Err(
				errnie.Validation,
				"broker: execution missing symbol",
				nil,
			))
			continue
		}

		if strings.EqualFold(execution.ExecType, "snapshot") {
			snapshot = true
		}

		if strings.EqualFold(execution.PositionStatus, "open") {
			snapshotSymbols[symbol] = struct{}{}
		}

		position, ok := desk.positions.Load(symbol)

		if ok {
			held := position.(*Position)
			held.Execution(&execution)

			if strings.EqualFold(execution.PositionStatus, "open") &&
				held.data.Mark.Rat().Sign() <= 0 {
				desk.hydrate(held)
			}

			continue
		}

		if !strings.EqualFold(execution.PositionStatus, "open") ||
			execution.LastQty <= 0 ||
			execution.AvgPrice.Rat().Sign() <= 0 {
			continue
		}

		position, _ = desk.positions.LoadOrStore(symbol, NewPosition(
			desk.private,
			&PositionData{
				Symbol:     symbol,
				Qty:        execution.LastQty,
				EntryPrice: execution.AvgPrice,
			},
		))

		position.(*Position).SetFeeRate(desk.takerRate(symbol))
		position.(*Position).executions = []*kraken.ExecutionData{&execution}
		position.(*Position).status = types.OPEN
		desk.hydrate(position.(*Position))

		desk.refreshStatus()
	}

	if snapshot {
		desk.positions.Range(func(key any, value any) bool {
			symbol := key.(string)

			if _, ok := snapshotSymbols[symbol]; ok {
				return true
			}

			position := value.(*Position)

			if position.status == types.PENDING && !position.closing {
				return true
			}

			position.status = types.CLOSED
			return true
		})
	}
}

/*
hydrate marks a restored open position with a current REST ticker snapshot.
*/
func (desk *Desk) hydrate(position *Position) {
	tickers, err := desk.public.Ticker([]string{position.data.Symbol})

	if err != nil {
		errnie.Error(err)
		return
	}

	for _, ticker := range tickers {
		if err := position.AddTicker(&ticker); err != nil {
			errnie.Error(err)
		}
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
	// A Buy sizes against the account balance, so it cannot proceed until the
	// account has hydrated. This is a hard precondition for opening a position
	// and is independent of the capacity status. (Sell has no such dependency.)
	if desk.balance == nil {
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"broker: account not hydrated",
			nil,
		))
	}

	// BUSY means normal and reserved slots are both full: no Buy of any kind is
	// accepted. READY and PRIORITY fall through — PRIORITY accepts only reserved
	// (opportunity) buys, enforced by the count check below.
	if desk.status == types.BUSY {
		return errnie.Err(
			errnie.NotAcceptable,
			"broker: max positions and max reserved reached",
			nil,
		)
	}

	openPositions := desk.OpenPositions()

	if openPositions >= desk.maxPositions && !opportunity {
		// At the normal cap (PRIORITY): only a high-value opportunity may take a
		// reserved slot; an ordinary buy is turned away.
		return errnie.Err(
			errnie.NotAcceptable,
			"broker: max positions reached",
			nil,
		)
	}

	if price.Rat().Sign() <= 0 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy price must be positive",
			nil,
		))
	}

	if math.IsNaN(fraction) || math.IsInf(fraction, 0) || fraction <= 0 || fraction > 1 {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy fraction must be within the quote balance",
			nil,
		))
	}

	_, quote, ok := strings.Cut(strings.TrimSpace(symbol), "/")
	quote = strings.TrimSpace(quote)

	if !ok || quote == "" {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"broker: buy symbol must include base and quote",
			nil,
		))
	}

	fractionRat := new(big.Rat).SetFloat64(fraction)
	priceRat := price.Rat()
	qty := 0.0
	quoteFound := false

	for _, balance := range *desk.balance {
		if strings.EqualFold(balance.Asset, quote) {
			quoteFound = true
			available := balance.Available.Rat()

			if available.Sign() <= 0 {
				return errnie.Error(errnie.Err(
					errnie.Validation,
					"broker: quote balance must be positive",
					nil,
				))
			}

			quoteSpend := new(big.Rat).Mul(available, fractionRat)
			qtyRat := new(big.Rat).Quo(quoteSpend, priceRat)
			qty, _ = qtyRat.Float64()
			break
		}
	}

	if !quoteFound {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"broker: quote balance not found",
			nil,
		))
	}

	if math.IsNaN(qty) || math.IsInf(qty, 0) || qty <= 0 {
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

	held := position.(*Position)
	held.SetFeeRate(desk.takerRate(symbol))

	if err := held.AddTicker(&kraken.TickerData{
		Symbol: symbol,
		Bid:    price,
		Ask:    price,
		Last:   price,
	}); err != nil {
		desk.positions.Delete(symbol)
		return err
	}

	return held.Enter()
}

func (desk *Desk) Sell(symbol string) (err error) {
	// Sell is never gated on capacity status. A close must always be allowed
	// through in every state — a full book (PRIORITY/BUSY) reclaims a slot by
	// exiting, and a protective or take-profit exit can never be blocked.
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
