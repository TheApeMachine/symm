package broker

import (
	"iter"
	"maps"
	"sync/atomic"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Balance owns the wallet map and reservation ledger for the Desk event loop.
Snapshots replace the entire asset map; updates require exact next-sequence.
*/
type Balance struct {
	status   atomic.Value
	api      *websocket.API
	data     map[string]*kraken.BalanceData
	holdings map[string]*types.Holding
	ledger   *Ledger
	quote    string
	ui       chan []byte
	sequence int64
}

/*
NewBalance constructs an empty wallet owner. Seed holdings are copied by value.
*/
func NewBalance(
	api *websocket.API,
	holdings []types.Holding,
	ui chan []byte,
	market config.MarketConfig,
) *Balance {
	balance := &Balance{
		api:      api,
		quote:    market.QuoteCurrency,
		data:     make(map[string]*kraken.BalanceData),
		holdings: make(map[string]*types.Holding),
		ledger:   NewLedger(),
		ui:       ui,
		sequence: 0,
	}
	balance.status.Store(types.INITIALIZING)

	for _, holding := range holdings {
		lot := holding
		balance.holdings[holding.Symbol] = &lot
	}

	return balance
}

/*
Ledger exposes the reservation ledger so Desk and Allocator share one owner.
*/
func (balance *Balance) Ledger() *Ledger {
	return balance.ledger
}

/*
Initialize subscribes the private balances channel when an API is attached.
*/
func (balance *Balance) Initialize() error {
	errnie.Info("initializing balance")

	if balance.api == nil {
		balance.status.Store(types.READY)
		return nil
	}

	balance.status.Store(types.PENDING)

	if errnie.Error(balance.api.SubscribeBalance()) != nil {
		balance.status.Store(types.ERROR)

		return errnie.Error(errnie.Err(
			errnie.Internal,
			"failed to subscribe to balance",
			nil,
		))
	}

	return nil
}

/*
Status reports wallet readiness.
*/
func (balance *Balance) Status() types.Status {
	status := balance.status.Load()

	if status == nil {
		return types.INITIALIZING
	}

	return status.(types.Status)
}

/*
Publish enqueues Frame() on the UI channel. Saturated channels drop the frame.
*/
func (balance *Balance) Publish() {
	frame := balance.Frame()

	if len(frame) == 0 || balance.ui == nil {
		return
	}

	select {
	case balance.ui <- frame:
	default:
	}
}

/*
BalanceAck applies one private balance frame. Snapshots replace the wallet map
and sequence; updates require exact next-sequence or a resync is requested.
*/
func (balance *Balance) BalanceAck(buf []byte) {
	incoming := kraken.NewBalance(buf)

	if errnie.Error(kraken.Validate(incoming)) != nil {
		return
	}

	if balance.data == nil {
		balance.data = make(map[string]*kraken.BalanceData)
	}

	if incoming.Type == "snapshot" {
		replaced := make(map[string]*kraken.BalanceData, len(incoming.Data))

		for index := range incoming.Data {
			row := incoming.Data[index]
			replaced[row.Asset] = cloneBalanceRow(&row)
		}

		balance.data = replaced
		balance.sequence = incoming.Sequence
		balance.syncWallet()
		balance.status.Store(types.READY)
		balance.Publish()

		return
	}

	if balance.sequence > 0 && incoming.Sequence != balance.sequence+1 {
		errnie.Error(errnie.Err(
			errnie.Validation,
			"balance: sequence gap requires snapshot resync",
			nil,
		))
		balance.resync()

		return
	}

	for index := range incoming.Data {
		row := incoming.Data[index]
		balance.data[row.Asset] = cloneBalanceRow(&row)
	}

	balance.sequence = incoming.Sequence
	balance.syncWallet()
	balance.status.Store(types.READY)
	balance.Publish()
}

func (balance *Balance) resync() {
	if balance.api == nil {
		return
	}

	errnie.Error(balance.api.SubscribeBalance())
}

func cloneBalanceRow(row *kraken.BalanceData) *kraken.BalanceData {
	cloned := *row

	if row.Balance != nil {
		cloned.Balance = row.Balance.Copy()
	}

	return &cloned
}

/*
Get returns a copied wallet row for symbol.
*/
func (balance *Balance) Get(symbol string) (*kraken.BalanceData, error) {
	if err := balance.validate(map[string]any{
		"symbol": symbol,
	}); err != nil {
		return nil, err
	}

	row, ok := balance.data[symbol]

	if !ok {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"balance not found for "+symbol,
			nil,
		))
	}

	return cloneBalanceRow(row), nil
}

/*
Holdings yields open non-quote inventory lots as value copies.
*/
func (balance *Balance) Holdings() iter.Seq[types.Holding] {
	return func(yield func(types.Holding) bool) {
		for symbol, holding := range balance.holdings {
			if symbol != holding.Symbol {
				continue
			}

			if holding.Status == types.CLOSED {
				continue
			}

			if !yield(*holding) {
				return
			}
		}
	}
}

/*
Holding returns a value copy of an open lot.
*/
func (balance *Balance) Holding(symbol string) (types.Holding, error) {
	holding, ok := balance.holdings[symbol]

	if ok && holding.Status == types.CLOSED {
		ok = false
	}

	if !ok {
		return types.Holding{}, errnie.Error(errnie.Err(
			errnie.NotFound,
			"holding not found for "+symbol,
			nil,
		))
	}

	return *holding, nil
}

/*
StoreHolding writes a lot into the wallet map (Desk enter / adopt path).
*/
func (balance *Balance) StoreHolding(holding *types.Holding) {
	balance.holdings[holding.Symbol] = holding
}

/*
DeleteHolding removes a pending lot that never filled.
*/
func (balance *Balance) DeleteHolding(symbol string) {
	delete(balance.holdings, symbol)
}

/*
LookupHolding returns the live lot pointer for Desk fill/mark paths.
*/
func (balance *Balance) LookupHolding(symbol string) (*types.Holding, bool) {
	holding, ok := balance.holdings[symbol]

	return holding, ok
}

/*
Symbol returns the canonical slash pair for a Kraken name via the SDK normalizer.
*/
func (balance *Balance) Symbol(pair string) string {
	return balance.api.Name(pair)
}

/*
TradeMatchesSymbol reports whether a REST trade-history pair belongs to symbol.
*/
func (balance *Balance) TradeMatchesSymbol(tradePair string, symbol string) bool {
	return balance.api.Name(tradePair) == balance.api.Name(symbol)
}

/*
Available reports whether free quote cash covers amount after live reservations.
*/
func (balance *Balance) Available(amount *decimal.Decimal) bool {
	if balance.validate(map[string]any{
		"amount": amount,
	}) != nil {
		return false
	}

	free, err := balance.FreeCash()

	if err != nil {
		errnie.Error(err)

		return false
	}

	return free.Sub(amount).Sign() >= 0
}

/*
FreeCash is exchange quote balance minus open cash reservations.
*/
func (balance *Balance) FreeCash() (*decimal.Decimal, error) {
	row, err := balance.Get(balance.quote)

	if err != nil {
		return nil, errnie.Error(errnie.Err(
			errnie.NotFound,
			"quote balance not found",
			err,
		))
	}

	reserved := balance.ledger.ReservedCash()

	return row.Balance.Copy().Sub(reserved), nil
}

/*
AssetAvailable returns sellable base qty after subtracting sell reservations.
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

	reserved := balance.ledger.ReservedAsset(asset)

	return row.Balance.Copy().Sub(reserved), nil
}

func (balance *Balance) validate(mandatory map[string]any) error {
	check := map[string]any{
		"data": balance.data,
	}

	maps.Copy(check, mandatory)

	return errnie.Error(errnie.Require(check))
}
