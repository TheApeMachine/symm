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
		balance.mu.Lock()
		balance.status = types.ERROR
		balance.mu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to balance",
			nil,
		))
	}

	balance.mu.Lock()
	balance.status = types.READY
	balance.mu.Unlock()
	balance.Publish()

	return nil
}

func (balance *Balance) Status() types.Status {
	balance.mu.Lock()
	defer balance.mu.Unlock()

	return balance.status
}

/*
Frame marshals the current quote snapshot, holdings, and stop surfaces for the
terminal. Nil when Balance is not ready to publish a coherent desk view.
Snapshot copy runs under mu; marshal runs unlocked so mark-path Publish does
not hold the lock across JSON encoding.
*/
func (balance *Balance) Frame() []byte {
	balance.mu.Lock()

	if balance.status != types.READY || balance.model == nil {
		balance.mu.Unlock()
		return nil
	}

	holdings := make([]*types.Holding, 0)
	stops := make([]map[string]any, 0)

	balance.holdings.Range(func(_, value any) bool {
		holding, ok := value.(*types.Holding)

		if !ok || holding == nil {
			return true
		}

		holdings = append(holdings, holding)

		if holding.Status == types.CLOSED || holding.Stoploss == nil {
			return true
		}

		if frame := holding.StopFrame(); frame != nil {
			stops = append(stops, frame)
		}

		return true
	})

	payload := datura.Map[any]{
		"balances": balance.snapshotLocked(),
		"holdings": holdings,
	}

	if len(stops) > 0 {
		payload["stops"] = stops
	}

	balance.mu.Unlock()

	return payload.Marshal()
}

/*
Publish enqueues a desk snapshot on the UI channel. Callers that must deliver
on websocket connect should Write Frame() directly — a saturated channel drops
this non-blocking send. Empty payloads (marshal failure) are never enqueued.
*/
func (balance *Balance) Publish() {
	if balance.ui == nil {
		return
	}

	if len(balance.ui) == cap(balance.ui) {
		return
	}

	frame := balance.Frame()

	if len(frame) == 0 {
		return
	}

	select {
	case balance.ui <- frame:
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
syncWallet mirrors non-quote exchange Balance onto Holding.Qty and Available
onto SellableQty. A lot closes only when total Balance is zero — Available
alone can be zero while inventory remains reserved on an open sell.
*/
func (balance *Balance) syncWallet() {
	if balance == nil || balance.model == nil {
		return
	}

	seen := make(map[string]struct{}, len(balance.model.Data))

	for _, row := range balance.model.Data {
		if row.Asset == "" || row.Asset == balance.quote {
			continue
		}

		symbol := row.Asset + "/" + balance.quote
		seen[symbol] = struct{}{}

		qty := row.Balance

		if qty == nil || qty.Sign() <= 0 {
			balance.closeHolding(symbol)
			continue
		}

		qty = qty.Copy()

		// Settled base inventory means the entry filled and wallet sync now owns
		// the cash; retire any lingering local claim so it cannot double-count.
		balance.consumeClaimsForSymbolLocked(symbol)

		sellable := decimal.NewFromInt64(0)

		if row.Available != nil {
			sellable = row.Available.Copy()
		}

		if value, ok := balance.holdings.Load(symbol); ok {
			holding := value.(*types.Holding)
			holding.Qty = qty
			holding.SellableQty = sellable
			holding.Asset = row.Asset
			holding.Status = types.OPEN

			continue
		}

		holding := types.NewHolding(context.Background(), symbol, qty)
		holding.Asset = row.Asset
		holding.SellableQty = sellable
		holding.Status = types.OPEN
		balance.holdings.Store(symbol, holding)
	}

	balance.holdings.Range(func(key, value any) bool {
		symbol, ok := key.(string)

		if !ok {
			return true
		}

		if _, present := seen[symbol]; present {
			return true
		}

		balance.closeHolding(symbol)

		return true
	})
}

/*
closeHolding transitions a wallet shell to CLOSED without deleting it.
*/
func (balance *Balance) closeHolding(symbol string) {
	if balance == nil || symbol == "" {
		return
	}

	value, ok := balance.holdings.Load(symbol)

	if !ok {
		return
	}

	holding := value.(*types.Holding)
	holding.Status = types.CLOSED

	if holding.Qty == nil || holding.Qty.Sign() != 0 {
		holding.Qty = decimal.NewFromFloat64(0)
	}

	holding.SellableQty = decimal.NewFromFloat64(0)
}

/*
Resync retains the last confirmed Kraken snapshot for risk-reducing inventory
access, marks quote capital unavailable, and requests a fresh snapshot. A
validated snapshot returns the balance to READY; new reservations cannot use
stale cash while resynchronization is pending.
*/
func (balance *Balance) Resync() error {
	if balance == nil {
		return nil
	}

	balance.mu.Lock()
	balance.status = types.PENDING
	api := balance.api
	balance.mu.Unlock()

	if api == nil {
		return nil
	}

	if err := api.SubscribeBalance(); err != nil {
		balance.mu.Lock()
		balance.status = types.ERROR
		balance.mu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"balance: SubscribeBalance failed during resync",
			err,
		))
	}

	return nil
}

func (balance *Balance) Get(symbol string) (*kraken.BalanceData, error) {
	if balance == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"balance model not available",
			nil,
		))
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()

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
Seed installs a wallet-backed open lot for fixtures. It upserts the base asset
into the live snapshot so Remember and Desk slot counts stay wallet-authoritative.
*/
func (balance *Balance) Seed(holding *types.Holding) {
	if balance == nil || holding == nil || holding.Symbol == "" {
		return
	}

	asset := holding.Asset

	if asset == "" {
		asset = strings.TrimSuffix(holding.Symbol, "/"+balance.quote)
	}

	if asset == "" || asset == balance.quote {
		return
	}

	holding.Asset = asset

	if holding.Qty == nil || holding.Qty.Sign() <= 0 {
		return
	}

	qty := holding.Qty.Copy()

	if balance.model == nil {
		balance.model = &kraken.Balance{Type: "snapshot", Data: []kraken.BalanceData{}}
		balance.status = types.READY
	}

	replaced := false

	for index := range balance.model.Data {
		if balance.model.Data[index].Asset != asset {
			continue
		}

		balance.model.Data[index].Balance = qty
		balance.model.Data[index].Available = qty.Copy()
		replaced = true
		break
	}

	if !replaced {
		balance.model.Data = append(balance.model.Data, kraken.BalanceData{
			Asset:     asset,
			Balance:   qty,
			Available: qty.Copy(),
		})
	}

	balance.holdings.Store(holding.Symbol, holding)
}

/*
Enrich merges durable recovery economics onto a wallet-backed open lot.
*/
func (balance *Balance) Enrich(symbol string, recovered types.Holding) {
	if balance == nil || symbol == "" {
		return
	}

	value, ok := balance.holdings.Load(symbol)

	if !ok {
		return
	}

	value.(*types.Holding).Enrich(recovered)
}

/*
Remember stores an open holding only when the live wallet still shows positive
qty for that asset. Recovery metadata must not invent inventory the exchange
has already flattened.
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

	// Wallet authority applies only after a live snapshot exists. Fixture
	// paths with a nil model may still seed holdings for unit tests.
	if balance.model != nil && !balance.walletHolds(holding) {
		return
	}

	balance.holdings.Store(holding.Symbol, holding)
}

/*
ModelReady reports whether a live balance snapshot has been applied.
*/
func (balance *Balance) ModelReady() bool {
	return balance != nil && balance.model != nil && balance.status == types.READY
}

/*
walletHolds reports whether the latest balance snapshot still carries positive
qty for the holding's base asset.
*/
func (balance *Balance) walletHolds(holding *types.Holding) bool {
	if balance.model == nil || holding == nil {
		return false
	}

	asset := holding.Asset

	if asset == "" {
		asset = strings.TrimSuffix(holding.Symbol, "/"+balance.quote)
	}

	if asset == "" || asset == balance.quote {
		return false
	}

	for _, row := range balance.model.Data {
		if row.Asset != asset {
			continue
		}

		return row.Balance != nil && row.Balance.Sign() > 0
	}

	return false
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
claimedQuoteLocked sums active local reservation amounts. Caller holds mu.
*/
func (balance *Balance) claimedQuoteLocked() *decimal.Decimal {
	sum := decimal.NewFromInt64(0)

	if balance == nil || len(balance.books) == 0 {
		return sum
	}

	for _, row := range balance.books {
		if row == nil || row.Amount == nil {
			continue
		}

		sum = sum.Add(row.Amount)
	}

	return sum
}

/*
effectiveAvailableLocked is exchange quote Available minus local claims.
Caller holds mu. Exchange rows are never mutated by Book/Release.
*/
func (balance *Balance) effectiveAvailableLocked() (*decimal.Decimal, error) {
	if balance == nil || balance.model == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote balance not found",
			nil,
		))
	}

	if balance.status != types.READY {
		return nil, errnie.Error(errnie.Err(
			errnie.Conflict,
			"quote balance unavailable while snapshot resync is pending",
			nil,
		))
	}

	for _, row := range balance.model.Data {
		if row.Asset != balance.quote || row.Available == nil {
			continue
		}

		return row.Available.Sub(balance.claimedQuoteLocked()), nil
	}

	return nil, errnie.Error(errnie.Err(
		errnie.NotFound,
		"quote balance not found",
		nil,
	))
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

	available, err := balance.effectiveAvailableLocked()

	if err != nil || available == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote balance not found",
			err,
		))
	}

	// A finite fixed-point product needs at most the sum of its operand scales.
	// Scale one is the minimum because the SDK's scale-zero banker rounding
	// misclassifies exact odd integers as half-way values.
	scale := max(int64(1), available.GetScale()+fraction.GetScale())

	return available.SetScale(scale).Mul(fraction), nil
}

/*
Available reports whether effective free quote cash covers amount after live
claims. Callers use it to refuse entries that would overdraw booked capital.
*/
func (balance *Balance) Available(amount *decimal.Decimal) bool {
	if amount == nil || balance == nil {
		return false
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()

	effective, err := balance.effectiveAvailableLocked()

	if err != nil || effective == nil {
		return false
	}

	return effective.Sub(amount).Sign() >= 0
}

/*
AssetAvailable returns the sellable qty for a base asset from the live wallet
snapshot. Missing rows are not found rather than silently zero.
*/
func (balance *Balance) AssetAvailable(asset string) (*decimal.Decimal, error) {
	if balance == nil || asset == "" {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"asset available balance missing",
			nil,
		))
	}

	row, err := balance.Get(asset)

	if err != nil {
		return nil, err
	}

	if row.Available == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"asset available balance missing for "+asset,
			nil,
		))
	}

	return row.Available.Copy(), nil
}

/*
AvailableCash returns effective unreserved quote capital: exchange Available
minus active local Book claims. Missing balance state is an error rather than
zero available cash.
*/
func (balance *Balance) AvailableCash() (*decimal.Decimal, error) {
	if balance == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote available balance missing",
			nil,
		))
	}

	balance.mu.Lock()
	defer balance.mu.Unlock()

	effective, err := balance.effectiveAvailableLocked()

	if err != nil || effective == nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote available balance missing",
			err,
		))
	}

	return effective.Copy(), nil
}

func (balance *Balance) validate(mandatory map[string]any) error {
	check := map[string]any{
		"model": balance.model,
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
