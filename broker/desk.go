package broker

import (
	"math"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type Desk struct {
	status         types.Status
	api            *websocket.API
	ui             chan []byte
	price          *Price
	balance        *Balance
	positions      *sync.Map
	maxPositions   int
	maxReserved    int
	publishPending atomic.Bool
}

func NewDesk(
	api *websocket.API,
	price *Price,
	balance *Balance,
	messages chan []byte,
) *Desk {
	return &Desk{
		status:       types.INITIALIZING,
		api:          api,
		ui:           messages,
		price:        price,
		balance:      balance,
		positions:    &sync.Map{},
		maxPositions: viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:  viper.GetViper().GetInt("trading.slots.reserved"),
	}
}

/*
Initialize waits for the wallet to report holdings, then loads open spot
inventory from trade history. The booter calls this once per boot.
*/
func (desk *Desk) Initialize() error {
	errnie.Info("initializing desk")
	feeSymbols := make([]string, 0)

	for holding := range desk.balance.Holdings() {
		if holding.Asset == desk.balance.quote || holding.Qty.Sign() <= 0 ||
			strings.Contains(holding.Asset, "/") {
			continue
		}

		feeSymbols = append(feeSymbols, desk.balance.Symbol(holding.Asset))
	}

	if len(feeSymbols) > 0 {
		if err := desk.price.GetFees(feeSymbols); err != nil {
			desk.status = types.ERROR

			return errnie.Error(errnie.Err(
				errnie.Internal,
				"failed to load fees for existing holdings",
				err,
			))
		}
	}

	history, err := desk.api.TradesHistory()

	if err != nil {
		desk.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to get trades history",
			err,
		))
	}

	for holding := range desk.balance.Holdings() {
		if holding.Asset == desk.balance.quote || holding.Qty.Sign() <= 0 {
			continue
		}

		if strings.Contains(holding.Asset, "/") {
			continue
		}

		fixed := desk.balance.Symbol(holding.Asset)
		holding.Symbol = fixed
		desk.balance.Update(fixed, holding)

		order := &spot.Order{
			Description: &spot.OrderDescription{
				Pair: fixed,
			},
			Volume: &holding.Qty,
			Price:  decimal.NewFromInt64(0),
		}

		/*
			Ask Price to calculate the complete current position value.

			Desk does not calculate:
			  - notionals
			  - fees
			  - PnL
			  - return percentage

			The ticker universe subscribes hundreds of symbols on boot, so
			this symbol's own ticker may not have arrived yet. That is not
			a reason to drop the holding: Hydrate below still confirms it
			against the wallet, and Position.TickerAck fills in Mark, PnL,
			and ReturnPct the moment this symbol's next ticker arrives.
		*/
		position := NewPosition(
			desk.api,
			desk.ui,
			desk.price,
			desk.balance,
			order,
		)

		desk.positions.Store(fixed, position)
		position.Hydrate(fixed, history)

		if position.Status() != types.OPEN {
			desk.positions.Delete(fixed)
			errnie.Error(errnie.Err(
				errnie.Validation,
				"failed to recover open position for "+fixed+"; holding skipped at boot",
				nil,
			))

			continue
		}

	}

	desk.status = types.READY
	return nil
}

func (desk *Desk) Publish() {
	select {
	case desk.ui <- datura.Map[any]{
		"balances": desk.balance.Snapshot(),
	}.Marshal():
	default:
	}
}

func (desk *Desk) Status() types.Status {
	return desk.status
}

func (desk *Desk) OpenPositions() int {
	count := 0

	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		if position.Stop == nil {
			return true
		}

		holding, err := desk.balance.Holding(position.Stop.Symbol)

		if err != nil {
			return true
		}

		if holding.Qty.Sign() > 0 || position.Status() == types.PENDING ||
			position.Status() == types.OPEN {
			count++
		}

		return true
	})

	return count
}

func (desk *Desk) Positions() []*Position {
	positions := make([]*Position, 0)

	desk.positions.Range(func(_, value any) bool {
		position := value.(*Position)

		if position.Stop == nil {
			return true
		}

		holding, err := desk.balance.Holding(position.Stop.Symbol)

		if err != nil || holding.Qty.Sign() <= 0 {
			return true
		}

		positions = append(positions, position)

		return true
	})

	return positions
}

func (desk *Desk) Buy(
	order spot.Order,
	pair kraken.InstrumentPair,
) error {
	if order.Description == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"order description required",
			nil,
		))
	}

	symbol := order.Description.Pair
	opportunity := order.Description.OrderType == "reserved"

	if desk.status != types.READY {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"desk not ready to buy",
			nil,
		))
	}

	openPositions := desk.OpenPositions()

	if openPositions >= desk.maxPositions+desk.maxReserved {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"desk at max positions and reserved",
			nil,
		))
	}

	if openPositions >= desk.maxPositions && !opportunity {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"desk at max positions",
			nil,
		))
	}

	if _, exists := desk.positions.Load(symbol); exists {
		return errnie.Error(errnie.Err(
			errnie.Forbidden,
			"position already exists for "+symbol,
			nil,
		))
	}

	notional := order.Volume.Float64()
	quantity, price, err := desk.quantity(symbol, notional, pair)

	if err != nil {
		return err
	}

	spotOrder := &spot.Order{
		Description: &spot.OrderDescription{
			Pair: symbol,
		},
		Volume: &quantity,
		Price:  &price,
	}

	position := NewPosition(
		desk.api,
		desk.ui,
		desk.price,
		desk.balance,
		spotOrder,
	)

	desk.positions.Store(symbol, position)

	desk.balance.Update(symbol, types.Holding{
		Symbol: symbol,
		Asset:  pair.Base,
		Order:  spotOrder,
		Qty:    *decimal.NewFromInt64(0),
	})

	if err := position.Enter(); err != nil {
		desk.positions.Delete(symbol)

		return err
	}

	return nil
}

/*
quantity converts proposed quote capital into a base quantity at the executable
ask, rounds down to Kraken's increment, and enforces both exchange minima.
*/
func (desk *Desk) quantity(
	symbol string,
	notional float64,
	pair kraken.InstrumentPair,
) (decimal.Decimal, decimal.Decimal, error) {
	ticker, err := desk.price.Get(symbol)

	if err != nil || ticker.Ask == nil || ticker.Ask.Sign() <= 0 {
		return decimal.Decimal{}, decimal.Decimal{}, errnie.Error(errnie.Err(
			errnie.NotFound, "executable ask not available for "+symbol, err,
		))
	}

	fee, err := desk.price.FeeFraction(symbol)

	if err != nil {
		return decimal.Decimal{}, decimal.Decimal{}, err
	}

	if pair.QtyIncrement <= 0 {
		return decimal.Decimal{}, decimal.Decimal{}, errnie.Error(errnie.Err(
			errnie.Validation, "quantity increment required for "+symbol, nil,
		))
	}

	unitCost := ticker.Ask.Float64() * (1 + fee.Float64())
	quantity := math.Floor((notional/unitCost)/pair.QtyIncrement) * pair.QtyIncrement

	if quantity < pair.QtyMin {
		return decimal.Decimal{}, decimal.Decimal{}, errnie.Error(errnie.Err(
			errnie.Forbidden, "proposed quantity below exchange minimum for "+symbol, nil,
		))
	}

	price := *ticker.Ask
	amount := decimal.NewFromFloat64(quantity).SetScale(int64(pair.QtyPrecision))
	orderNotional := desk.price.Notional(price, *amount)

	if pair.CostMin.Float64() > 0 && orderNotional.Cmp(&pair.CostMin) < 0 {
		return decimal.Decimal{}, decimal.Decimal{}, errnie.Error(errnie.Err(
			errnie.Forbidden, "proposed notional below exchange minimum for "+symbol, nil,
		))
	}

	return *amount, price, nil
}

/*
Sell submits the selected exit through the Position already associated with
the order symbol, preserving one lifecycle across entry and exit.
*/
func (desk *Desk) Sell(order spot.Order) error {
	if order.Description == nil || order.Volume == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"order description and volume required",
			nil,
		))
	}

	symbol := order.Description.Pair
	position, ok := desk.positions.Load(symbol)

	if !ok || position == nil {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"position not found for "+symbol,
			nil,
		))
	}

	return position.(*Position).Exit(order.Description.Type, *order.Volume)
}

func (desk *Desk) Close() error {
	return nil
}
