package broker

import (
	"iter"
	"maps"
	"slices"
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
	data     *sync.Map
	holdings *sync.Map
	quote    string
	ui       chan []byte
	sequence int64
}

func NewBalance(api *websocket.API, holdings []types.Holding, ui chan []byte) *Balance {
	balance := &Balance{
		status:   types.INITIALIZING,
		api:      api,
		quote:    viper.GetViper().GetString("market.quote_currency"),
		data:     &sync.Map{},
		holdings: &sync.Map{},
		ui:       ui,
		sequence: 0,
	}

	for _, holding := range holdings {
		balance.holdings.Store(holding.Symbol, &holding)
	}

	return balance
}

func (balance *Balance) Initialize() error {
	errnie.Info("initializing balance")

	if balance.api == nil {
		balance.status = types.READY
		return nil
	}

	balance.status = types.PENDING

	if errnie.Error(balance.api.SubscribeBalance()) != nil {
		balance.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to balance",
			nil,
		))
	}

	return nil
}

func (balance *Balance) Status() types.Status {
	return balance.status
}

/*
Publish enqueues a desk snapshot on the UI channel. Callers that must deliver
on websocket connect should Write Frame() directly — a saturated channel drops
this non-blocking send. Empty payloads (marshal failure) are never enqueued.
*/
func (balance *Balance) Publish() {
	if balance.data == nil {
		return
	}

	balances := make([]kraken.BalanceData, 0)

	balance.data.Range(func(key, value any) bool {
		balances = append(balances, *value.(*kraken.BalanceData))
		return true
	})

	select {
	case balance.ui <- datura.Map[any]{
		"balances": balances,
		"holdings": slices.Collect(balance.Holdings()),
	}.Marshal():
	default:
	}
}

func (balance *Balance) BalanceAck(buf []byte) {
	incoming := kraken.NewBalance(buf)

	if errnie.Error(kraken.Validate(incoming)) != nil {
		return
	}

	if balance.data == nil || incoming.Type == "snapshot" {
		for _, data := range incoming.Data {
			balance.data.Store(data.Asset, &data)
		}

		balance.status = types.READY
		balance.Publish()

		return
	}

	if incoming.Sequence < balance.sequence {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"balance: older sequence received",
			nil,
		))

		return
	}

	for _, update := range incoming.Data {
		replaced := false

		balance.data.Range(func(key, value any) bool {
			if key.(string) != update.Asset {
				return true
			}

			balance.data.Store(key, &update)
			replaced = true
			return false
		})

		if !replaced {
			balance.data.Store(update.Asset, &update)
		}
	}

	balance.sequence = incoming.Sequence
	balance.status = types.READY

	balance.Publish()
}

func (balance *Balance) Get(symbol string) (*kraken.BalanceData, error) {
	if err := balance.validate(map[string]any{
		"symbol": symbol,
	}); err != nil {
		return nil, err
	}

	if balance.data == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"balance data not available",
			nil,
		))
	}

	value, ok := balance.data.Load(symbol)

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"balance not found for "+symbol,
			nil,
		))
	}

	return value.(*kraken.BalanceData), nil
}

/*
Holdings returns non-quote spot wallet balances that represent open inventory.
*/
func (balance *Balance) Holdings() iter.Seq[types.Holding] {
	return func(yield func(types.Holding) bool) {
		balance.holdings.Range(func(key, value any) bool {
			holding := value.(*types.Holding)

			if key.(string) != holding.Symbol {
				return true
			}

			if holding.Status == types.CLOSED {
				return true
			}

			return yield(*holding)
		})
	}
}

/*
Holding returns the holding for a given symbol.
*/
func (balance *Balance) Holding(symbol string) (types.Holding, error) {
	value, ok := balance.holdings.Load(symbol)

	// A closed lot is no longer inventory: Holdings() already skips it, so the
	// single-symbol lookup must agree and report the lot as gone rather than
	// handing back a flat, exited shell that reads as an open position.
	if ok && value.(*types.Holding).Status == types.CLOSED {
		ok = false
	}

	if !ok {
		return types.Holding{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			"holding not found for "+symbol,
			nil,
		))
	}

	return *value.(*types.Holding), nil
}

/*
Symbol returns the canonical slash pair for a Kraken name via the SDK
normalizer (NEARUSD → NEAR/USD). Quote-suffix trimming is not used — that
mis-splits assets whose codes contain the quote (USDC, PYUSD, …).
*/
func (balance *Balance) Symbol(pair string) string {
	return balance.api.Name(pair)
}

/*
TradeMatchesSymbol reports whether a REST trade-history pair belongs to the
normalized slash symbol used throughout symm.
*/
func (balance *Balance) TradeMatchesSymbol(tradePair string, symbol string) bool {
	return balance.api.Name(tradePair) == balance.api.Name(symbol)
}

/*
reservation resolves either a fixed amount or a fraction of effective Available.
Caller holds mu when fraction sizing needs the live claim ledger.
*/
func (balance *Balance) reservation(
	amount, fraction *decimal.Decimal,
) (*decimal.Decimal, error) {
	if amount != nil {
		if amount.Sign() <= 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"reserve amount must be positive",
				nil,
			))
		}

		return amount.Copy(), nil
	}

	if fraction == nil || fraction.Sign() <= 0 {
		return nil, errnie.Error(errnie.Err(
			errnie.Validation,
			"reserve requires amount or positive fraction",
			nil,
		))
	}

	available, err := balance.Get(balance.quote)

	if err != nil || available == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"available cash not found",
			err,
		))
	}

	// A finite fixed-point product needs at most the sum of its operand scales.
	// Scale one is the minimum because the SDK's scale-zero banker rounding
	// misclassifies exact odd integers as half-way values.
	cash := available.Balance.Copy()
	scale := max(int64(1), cash.GetScale()+fraction.GetScale())

	return cash.Copy().SetScale(scale).Mul(fraction), nil
}

/*
Available reports whether effective free quote cash covers amount after live
claims. Callers use it to refuse entries that would overdraw booked capital.
*/
func (balance *Balance) Available(amount *decimal.Decimal) bool {
	if balance.validate(map[string]any{
		"amount": amount,
	}) != nil {
		return false
	}

	effective, err := balance.Get(balance.quote)

	if err != nil {
		errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote balance not found",
			err,
		))

		return false
	}

	return effective.Balance.Copy().Sub(amount).Sign() >= 0
}

/*
AssetAvailable returns the sellable qty for a base asset from the live wallet
snapshot. Missing rows are not found rather than silently zero.
*/
func (balance *Balance) AssetAvailable(asset string) (*decimal.Decimal, error) {
	if err := balance.validate(map[string]any{
		"asset": asset,
	}); err != nil {
		return nil, err
	}

	row, err := balance.Get(asset)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"asset available balance missing for "+asset,
			err,
		))
	}

	return row.Balance.Copy(), nil
}

func (balance *Balance) validate(mandatory map[string]any) error {
	check := map[string]any{
		"data": balance.data,
	}

	maps.Copy(check, mandatory)
	return errnie.Error(errnie.Require(check))
}
