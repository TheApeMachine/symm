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
	status          types.Status
	private         websocket.Conn
	balance         *kraken.BalanceDataSlice
	positions       *sync.Map
	maxPositions    int
	maxReserved     int
	feeSchedule     *sync.Map
	fallbackFeeRate float64
}

func NewDesk(
	private, public websocket.Conn, messages chan []byte,
) *Desk {
	desk := &Desk{
		status:       types.INITIALIZING,
		private:      private,
		positions:    &sync.Map{},
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
		feeSchedule:  &sync.Map{},
	}

	orders := NewOrders(desk, messages)

	private.On("balances", NewBalances(desk, messages).On)
	private.On("executions", NewExecutions(desk, messages).On)
	private.On("orders", orders.On)
	private.On("add_order", orders.Ack)
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

func (desk *Desk) takerRate(symbol string) float64 {
	schedule, ok := desk.feeSchedule.Load(symbol)

	if !ok {
		if desk.fallbackFeeRate <= 0 ||
			math.IsNaN(desk.fallbackFeeRate) ||
			math.IsInf(desk.fallbackFeeRate, 0) {
			errnie.Error(errnie.Err(
				errnie.NotFound, "schedule not found for "+symbol, nil,
			))
		}

		return desk.fallbackFeeRate
	}

	switch val := schedule.(type) {
	case kraken.FeeRates:
		return val.Taker
	case map[string]kraken.FeeRates:
		return val["fee"].Taker
	default:
		return desk.fallbackFeeRate
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

	openPositions := desk.OpenPositions()

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

func (desk *Desk) Sell(symbol string) error {
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
	return nil
}
