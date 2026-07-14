package broker

import (
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

type SpotHolding struct {
	Asset string
	Qty   decimal.Decimal
}

type Balance struct {
	status types.Status
	api    *websocket.API
	ui     chan []byte
	model  *kraken.Balance
	quote  string
}

func NewBalance(api *websocket.API, ui chan []byte) *Balance {
	balance := &Balance{
		status: types.INITIALIZING,
		api:    api,
		ui:     ui,
		quote:  viper.GetString("market.quote_currency"),
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
	balance.model = kraken.NewBalance(buf)

	if errnie.Error(kraken.Validate(balance.model)) != nil {
		return
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
func (balance *Balance) Holdings() []SpotHolding {
	if balance.model == nil {
		return nil
	}

	holdings := make([]SpotHolding, 0)

	for _, balanceData := range balance.model.Data {
		holdings = append(holdings, SpotHolding{
			Asset: balanceData.Asset,
			Qty:   balanceData.Balance,
		})
	}

	return holdings
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
