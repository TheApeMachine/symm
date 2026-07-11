package broker

import (
	"math"
	"math/big"
	"strings"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Desk struct {
	status       types.Status
	private      websocket.Conn
	price        *Price
	balance      *kraken.BalanceDataSlice
	positions    *sync.Map
	dailyLoss    *DailyLoss
	maxPositions int
	maxReserved  int
}

func (desk *Desk) SetPrice(price *Price) {
	desk.price = price
}

func NewDesk(
	private, public websocket.Conn, messages chan []byte,
) *Desk {
	desk := &Desk{
		status:       types.INITIALIZING,
		private:      private,
		positions:    &sync.Map{},
		dailyLoss:    NewDailyLoss(),
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
	}

	orders := NewOrders(desk, messages)

	private.On("balances", NewBalances(desk, messages).On)
	private.On("executions", NewExecutions(desk, messages).On)
	private.On("orders", orders.On)
	private.On("add_order", orders.Ack)
	private.On("cancel_order", orders.Ack)
	public.On("ticker", NewMark(desk, messages).On)

	return desk
}

func (desk *Desk) Status() types.Status {
	if desk.balance == nil {
		return types.INITIALIZING
	}

	return desk.status
}

func (desk *Desk) refreshStatus() {
	open := desk.ExposureSlots()

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

	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		if position.exposed &&
			(position.status == types.OPEN || position.status == types.PARTIAL) {
			count++
		}

		return true
	})

	return count
}

func (desk *Desk) PendingCount() int {
	count := 0

	desk.positions.Range(func(_, value any) bool {
		if value.(*Position).status == types.PENDING {
			count++
		}

		return true
	})

	return count
}

func (desk *Desk) ExposureSlots() int {
	return desk.OpenPositions() + desk.PendingCount()
}

func (desk *Desk) Positions() []*Position {
	positions := make([]*Position, 0, desk.OpenPositions())

	desk.positions.Range(func(_, value any) bool {
		positions = append(positions, value.(*Position))
		return true
	})

	return positions
}

func (desk *Desk) takerRate(symbol string) (float64, bool) {
	if desk.price == nil {
		return 0, false
	}

	return desk.price.fee(symbol)
}

func (desk *Desk) Holdings() map[string]PositionData {
	holdings := map[string]PositionData{}

	desk.positions.Range(func(key any, value any) bool {
		position := value.(*Position)

		if position.exposed {
			holdings[key.(string)] = *position.data
		}

		return true
	})

	return holdings
}

/*
releasePosition folds a fully closed position's realized PnL into the
desk's daily loss tracker before removing it from the live position map.
Positions that never reached exposure carry a zero PnL, so recording is
harmless for rejected entries.
*/
func (desk *Desk) releasePosition(symbol string, position *Position) {
	desk.dailyLoss.Record(position.data.PnL)
	desk.positions.Delete(symbol)
}

func (desk *Desk) Buy(
	symbol string,
	fraction float64,
	price decimal.Decimal,
	opportunity bool,
) error {
	if desk.balance == nil {
		return errnie.Error(errnie.Err(
			errnie.Conflict,
			"broker: account not hydrated",
			nil,
		))
	}

	if desk.status == types.BUSY {
		return errnie.Err(
			errnie.NotAcceptable,
			"broker: max positions and max reserved reached",
			nil,
		)
	}

	if maxDailyLoss := viper.GetFloat64("live.max_daily_loss"); desk.dailyLoss.Exceeds(maxDailyLoss) {
		return errnie.Err(
			errnie.NotAcceptable,
			"broker: max_daily_loss breached, new entries blocked for today",
			nil,
		)
	}

	openPositions := desk.ExposureSlots()

	if openPositions >= desk.maxPositions && !opportunity {
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
			available := new(big.Rat).Sub(
				balance.Available.Rat(),
				desk.reservedQuote(quote),
			)

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

	if maxNotional := viper.GetFloat64("live.max_order_notional"); maxNotional > 0 {
		notionalRat := new(big.Rat).Mul(new(big.Rat).SetFloat64(qty), priceRat)

		if notionalRat.Cmp(new(big.Rat).SetFloat64(maxNotional)) > 0 {
			return errnie.Err(
				errnie.NotAcceptable,
				"broker: order notional exceeds configured max_order_notional",
				nil,
			)
		}
	}

	if desk.price != nil {
		if err := desk.price.Preflight(symbol, qty); err != nil {
			return err
		}
	}

	feeRate, ok := desk.takerRate(symbol)

	if !ok {
		return errnie.Err(
			errnie.NotFound,
			"broker: TradeVolume taker fee missing for "+symbol,
			nil,
		)
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
	held.SetFeeRate(feeRate)

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

func (desk *Desk) Sell(symbol string) error {
	position, ok := desk.positions.Load(symbol)

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"position not found for symbol",
			nil,
		))
	}

	held := position.(*Position)

	if !held.exposed {
		return errnie.Err(
			errnie.NotAcceptable,
			"position has no executed exposure",
			nil,
		)
	}

	return held.Exit()
}

func (desk *Desk) reservedQuote(quote string) *big.Rat {
	reserved := new(big.Rat)

	desk.positions.Range(func(_ any, value any) bool {
		position := value.(*Position)

		if position.status != types.PENDING ||
			position.closing ||
			position.exposed {
			return true
		}

		_, positionQuote, ok := strings.Cut(position.data.Symbol, "/")

		if ok && strings.EqualFold(positionQuote, quote) {
			quantity := new(big.Rat).SetFloat64(position.data.Qty)
			reserved.Add(
				reserved,
				new(big.Rat).Mul(position.data.EntryPrice.Rat(), quantity),
			)
		}

		return true
	})

	return reserved
}

func (desk *Desk) Close() error {
	return nil
}
