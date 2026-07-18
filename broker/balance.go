package broker

import (
	"context"
	"iter"
	"maps"
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
	model    *kraken.Balance
	holdings *sync.Map
	books    map[string]*Reservation
	quote    string
	ui       chan []byte
	mu       sync.Mutex
}

func NewBalance(api *websocket.API, holdings []types.Holding, ui chan []byte) *Balance {
	balance := &Balance{
		status:   types.INITIALIZING,
		api:      api,
		quote:    viper.GetViper().GetString("market.quote_currency"),
		holdings: &sync.Map{},
		books:    map[string]*Reservation{},
		ui:       ui,
	}

	for _, holding := range holdings {
		balance.holdings.Store(holding.Symbol, &holding)
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
	balance.Publish()

	return nil
}

func (balance *Balance) Status() types.Status {
	return balance.status
}

/*
Frame marshals the current quote snapshot and open holdings for the terminal.
Nil when Balance is not ready to publish a coherent desk view.
*/
func (balance *Balance) Frame() []byte {
	balance.mu.Lock()
	defer balance.mu.Unlock()

	if balance.status != types.READY || balance.model == nil {
		return nil
	}

	positions := make([]*types.Holding, 0)

	balance.holdings.Range(func(key, value any) bool {
		holding := value.(*types.Holding)
		positions = append(positions, holding)
		return true
	})

	return datura.Map[any]{
		"balances":  balance.snapshotLocked(),
		"positions": positions,
	}.Marshal()
}

/*
Publish enqueues a desk snapshot on the UI channel. Callers that must deliver
on websocket connect should Write Frame() directly — a saturated channel drops
this non-blocking send.
*/
func (balance *Balance) Publish() {
	if balance.ui == nil || balance.status != types.READY || balance.model == nil {
		return
	}

	if len(balance.ui) == cap(balance.ui) {
		return
	}

	select {
	case balance.ui <- balance.Frame():
	default:
	}
}

/*
Snapshot returns the quote-currency accounting row.
*/
func (balance *Balance) Snapshot() []datura.Map[any] {
	balance.mu.Lock()
	defer balance.mu.Unlock()

	return balance.snapshotLocked()
}

func (balance *Balance) snapshotLocked() []datura.Map[any] {
	balances := make([]datura.Map[any], 0, 1)

	if balance.model == nil {
		return balances
	}

	for _, balanceData := range balance.model.Data {
		if balanceData.Asset != balance.quote {
			continue
		}

		if balanceData.Balance == nil || balanceData.Available == nil {
			continue
		}

		reserved := 0.0

		if balanceData.Reserved != nil {
			reserved = balanceData.Reserved.Float64()
		}

		balances = append(balances, datura.Map[any]{
			"asset":     balanceData.Asset,
			"balance":   balanceData.Balance.Float64(),
			"available": balanceData.Available.Float64(),
			"reserved":  reserved,
		})
	}

	return balances
}

func (balance *Balance) BalanceAck(buf []byte) {
	publish := false

	balance.mu.Lock()

	incoming := kraken.NewBalance(buf)

	if errnie.Error(kraken.Validate(incoming)) != nil {
		balance.mu.Unlock()
		return
	}

	if balance.model == nil || incoming.Type == "snapshot" {
		balance.model = incoming
		balance.status = types.READY
		balance.syncWallet()
		publish = true
		balance.mu.Unlock()

		if publish {
			balance.Publish()
		}

		return
	}

	if incoming.Sequence < balance.model.Sequence {
		balance.mu.Unlock()
		return
	}

	if balance.model.Sequence > 0 &&
		incoming.Sequence > balance.model.Sequence+1 {

		balance.mu.Unlock()

		errnie.Error(errnie.Err(
			errnie.Validation,
			"balance: sequence gap; waiting for snapshot resync",
			nil,
		))

		errnie.Error(balance.Resync())

		return
	}

	for _, update := range incoming.Data {
		replaced := false

		for index := range balance.model.Data {
			if balance.model.Data[index].Asset != update.Asset {
				continue
			}

			balance.model.Data[index] = update
			replaced = true
			break
		}

		if !replaced {
			balance.model.Data = append(balance.model.Data, update)
		}
	}

	balance.model.Sequence = incoming.Sequence
	balance.model.Timestamp = incoming.Timestamp
	balance.status = types.READY
	balance.syncWallet()
	balance.mu.Unlock()
	balance.Publish()
}

/*
syncWallet mirrors non-quote asset balances onto open Holding rows.
*/
func (balance *Balance) syncWallet() {
	if balance == nil || balance.model == nil {
		return
	}

	for _, row := range balance.model.Data {
		if row.Asset == "" || row.Asset == balance.quote {
			continue
		}

		symbol := row.Asset + "/" + balance.quote

		if row.Balance == nil || row.Balance.Sign() <= 0 {
			balance.holdings.Delete(symbol)
			continue
		}

		qty := row.Balance.Copy()

		if value, ok := balance.holdings.Load(symbol); ok {
			holding := value.(*types.Holding)
			holding.Qty = qty
			holding.Asset = row.Asset

			if holding.Status == types.CLOSED || holding.Status == "" {
				holding.Status = types.OPEN
			}

			continue
		}

		holding := types.NewHolding(context.Background(), symbol, qty)
		holding.Asset = row.Asset
		holding.Status = types.OPEN
		balance.holdings.Store(symbol, holding)
	}
}

/*
Resync clears cached Kraken balance state and requests a fresh snapshot
subscription after reconciliation detects missing or inconsistent holdings.
*/
func (balance *Balance) Resync() error {
	balance.model = nil
	balance.status = types.PENDING

	if err := balance.api.SubscribeBalance(); err != nil {
		balance.status = types.ERROR

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"balance: SubscribeBalance failed during resync",
			err,
		))
	}

	return nil
}

func (balance *Balance) Get(symbol string) (*kraken.BalanceData, error) {
	if balance.model == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"balance model not available",
			nil,
		))
	}

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
Remember stores an open holding when restart recovery still knows the lot but
the wallet map lost its shell.
*/
func (balance *Balance) Remember(holding *types.Holding) {
	if balance == nil || holding == nil || holding.Symbol == "" {
		return
	}

	if holding.Status == types.CLOSED {
		return
	}

	if _, exists := balance.holdings.Load(holding.Symbol); exists {
		return
	}

	balance.holdings.Store(holding.Symbol, holding)
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

/*
Reserve moves quote capital from Available into Reserved so concurrent sizing
cannot double-spend the same cash. Pass amount for a fixed reservation, or
nil amount with fraction to reserve that share of current Available. When
rollback is true the same amount is released back to Available.
*/
func (balance *Balance) Reserve(
	amount, fraction *decimal.Decimal, rollback bool,
) (*decimal.Decimal, error) {
	balance.mu.Lock()
	defer balance.mu.Unlock()

	if err := balance.validate(map[string]any{
		"model": balance.model,
	}); err != nil {
		return nil, err
	}

	reserved, err := balance.reservation(amount, fraction)

	if err != nil {
		return nil, err
	}

	for index := range balance.model.Data {
		row := &balance.model.Data[index]

		if row.Asset != balance.quote || row.Available == nil {
			continue
		}

		if row.Reserved == nil {
			row.Reserved = decimal.NewFromInt64(0)
		}

		if rollback {
			row.Available = row.Available.Add(reserved)
			row.Reserved = row.Reserved.Sub(reserved)

			return reserved.Copy(), nil
		}

		if row.Available.Sub(reserved).Sign() < 0 {
			return nil, errnie.Error(errnie.Err(
				errnie.Validation,
				"insufficient available balance to reserve",
				nil,
			))
		}

		row.Available = row.Available.Sub(reserved)
		row.Reserved = row.Reserved.Add(reserved)

		return reserved.Copy(), nil
	}

	return nil, errnie.Error(errnie.Err(
		errnie.NotFound,
		"quote balance not found",
		nil,
	))
}

/*
reservation resolves either a fixed amount or a fraction of Available into the
decimal that will move between Available and Reserved.
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

	row, err := balance.Get(balance.quote)

	if err != nil || row.Available == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote balance not found",
			err,
		))
	}

	// Kraken Mul truncates the right factor to the left scale; lift first so
	// max_fraction 0.20 against an integer Available does not collapse to 0.
	scale := row.Available.GetScale()

	if fraction.GetScale() > scale {
		scale = fraction.GetScale()
	}

	scale += 8

	return row.Available.SetScale(scale).Mul(fraction.SetScale(scale)), nil
}

func (balance *Balance) Available(amount *decimal.Decimal) bool {
	if amount == nil {
		return false
	}

	if balance.model == nil {
		return false
	}

	for _, balanceData := range balance.model.Data {
		if balanceData.Asset != balance.quote || balanceData.Available == nil {
			continue
		}

		return balanceData.Available.Sub(amount).Sign() >= 0
	}

	return false
}

/*
AvailableCash returns unreserved quote-currency capital as a decimal so
Allocator can size lots without crossing through float64.
Missing balance state is an error rather than zero available cash.
*/
func (balance *Balance) AvailableCash() (*decimal.Decimal, error) {
	row, err := balance.Get(balance.quote)

	if err != nil {
		return nil, err
	}

	if row.Available == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote available balance missing",
			nil,
		))
	}

	return row.Available.Copy(), nil
}

/*
AvailableQuote returns the unreserved quote-currency capital as float64 for
Decide slot budgets that still consume float decision fields.
*/
func (balance *Balance) AvailableQuote() (float64, error) {
	cash, err := balance.AvailableCash()

	if err != nil {
		return 0, err
	}

	return cash.Float64(), nil
}

func (balance *Balance) validate(mandatory map[string]any) error {
	check := map[string]any{
		"model": balance.model,
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
