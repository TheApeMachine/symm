package broker

import (
	"iter"
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

type Balance struct {
	status   types.Status
	api      *websocket.API
	ui       chan []byte
	model    *kraken.Balance
	holdings *sync.Map
	quote    string
}

func NewBalance(api *websocket.API, ui chan []byte) *Balance {
	balance := &Balance{
		status:   types.INITIALIZING,
		api:      api,
		ui:       ui,
		quote:    viper.GetString("market.quote_currency"),
		holdings: &sync.Map{},
	}

	return balance
}

func (balance *Balance) Initialize() error {
	errnie.Info("initializing balance")

	balance.api.On("balances", balance.BalanceAck)

	if errnie.Error(balance.api.SubscribeBalance()) != nil {
		balance.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to balance",
			nil,
		))
	}

	balance.status = types.READY
	return nil
}

func (balance *Balance) Status() types.Status {
	return balance.status
}

func (balance *Balance) Publish() {
	if balance.status != types.READY {
		return
	}

	balance.ui <- datura.Map[any]{
		"balances": balance.Snapshot(),
	}.Marshal()
}

/*
Snapshot returns the quote-currency accounting row.
*/
func (balance *Balance) Snapshot() []datura.Map[any] {
	balances := make([]datura.Map[any], 0, 1)

	if balance.model == nil {
		return balances
	}

	for _, balanceData := range balance.model.Data {
		if balanceData.Asset != balance.quote {
			continue
		}

		balances = append(balances, datura.Map[any]{
			"asset":     balanceData.Asset,
			"balance":   balanceData.Balance.Float64(),
			"available": balanceData.Available.Float64(),
			"reserved":  balanceData.Reserved.Float64(),
		})
	}

	return balances
}

func (balance *Balance) BalanceAck(buf []byte) {
	incoming := kraken.NewBalance(buf)

	if errnie.Error(kraken.Validate(incoming)) != nil {
		return
	}

	if balance.model == nil || incoming.Type == "snapshot" {
		balance.model = incoming
	} else {
		for _, update := range incoming.Data {
			found := false

			for index := range balance.model.Data {
				if balance.model.Data[index].Asset != update.Asset {
					continue
				}

				balance.model.Data[index] = update
				found = true
				break
			}

			if !found {
				balance.model.Data = append(balance.model.Data, update)
			}
		}

		balance.model.Sequence = incoming.Sequence
		balance.model.Timestamp = incoming.Timestamp
	}

	for _, balanceData := range incoming.Data {
		assetHolding := types.Holding{
			Asset: balanceData.Asset,
			Qty:   balanceData.Balance,
		}

		if value, ok := balance.holdings.Load(balanceData.Asset); ok {
			assetHolding = value.(types.Holding)
			assetHolding.Qty = balanceData.Balance
		}

		balance.Update(balanceData.Asset, assetHolding)

		if balanceData.Asset == balance.quote {
			continue
		}

		symbol := balance.Symbol(balanceData.Asset)
		value, ok := balance.holdings.Load(symbol)

		if !ok {
			continue
		}

		holding := value.(types.Holding)
		holding.Symbol = symbol
		holding.Asset = balanceData.Asset
		holding.Qty = balanceData.Balance

		if holding.Order != nil {
			holding.Order.Volume = holding.Qty
		}

		balance.Update(symbol, holding)
	}

	balance.status = types.READY
	balance.Publish()
}

func (balance *Balance) Get(symbol string) (*kraken.BalanceData, error) {
	for _, balanceData := range balance.model.Data {
		if balanceData.Asset == symbol {
			return &balanceData, nil
		}
	}

	return nil, errnie.Error(errnie.Err(
		errnie.NotFound,
		"balance not found for "+symbol,
		nil,
	))
}

/*
Holdings returns non-quote spot wallet balances that represent open inventory.
*/
func (balance *Balance) Holdings() iter.Seq[types.Holding] {
	return func(yield func(types.Holding) bool) {
		balance.holdings.Range(func(key, value any) bool {
			holding := value.(types.Holding)

			if key.(string) != holding.Asset {
				return true
			}

			return yield(holding)
		})
	}
}

/*
Holding returns the holding for a given symbol.
*/
func (balance *Balance) Holding(symbol string) (types.Holding, error) {
	value, ok := balance.holdings.Load(symbol)

	if !ok {
		return types.Holding{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			"holding not found for "+symbol,
			nil,
		))
	}

	return value.(types.Holding), nil
}

/*
Update updates the holding for a given symbol.
*/
func (balance *Balance) Update(symbol string, holding types.Holding) {
	if symbol != "" {
		holding.Symbol = symbol
	}

	if holding.Asset != "" {
		balance.holdings.Store(holding.Asset, holding)
	}

	if symbol != "" {
		balance.holdings.Store(symbol, holding)
	}
}

/*
Symbol normalizes a compact trade-history pair (e.g. "BTCUSD") into the
slash-delimited symbol form (e.g. "BTC/USD") used everywhere else in
symm: WS v2 ticker/book/instrument symbols, and Price's cache keys.

It trims the quote currency as a suffix rather than replacing every
occurrence, since an asset code that itself contains the quote code
(USDC, USDT, PYUSD, ... against a USD quote) would otherwise lose its
own quote substring too.

If pair already carries a slash, it is assumed to be normalized and is
returned unchanged.
*/
func (balance *Balance) Symbol(pair string) string {
	if strings.Contains(pair, "/") {
		return pair
	}

	base := strings.TrimSuffix(pair, balance.quote)

	return base + "/" + balance.quote
}

/*
TradeMatchesSymbol reports whether a REST trade-history pair belongs to the
normalized slash symbol used throughout symm.
*/
func (balance *Balance) TradeMatchesSymbol(tradePair string, symbol string) bool {
	if balance.Symbol(tradePair) == symbol {
		return true
	}

	base := strings.TrimSuffix(symbol, "/"+balance.quote)
	compact := base + balance.quote

	return tradePair == compact || tradePair == base
}

func (balance *Balance) Available(amount decimal.Decimal) bool {
	for _, balanceData := range balance.model.Data {
		if balanceData.Asset == balance.quote {
			return balanceData.Available.Sub(&amount).Sign() >= 0
		}
	}

	return false
}

/*
AvailableQuote returns the unreserved quote-currency capital strategy may
allocate. Missing balance state is an error rather than zero available cash.
*/
func (balance *Balance) AvailableQuote() (float64, error) {
	if balance.model == nil {
		return 0, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote balance not available",
			nil,
		))
	}

	row, err := balance.Get(balance.quote)

	if err != nil {
		return 0, err
	}

	return row.Available.Float64(), nil
}
