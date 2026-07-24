package broker

import (
	"iter"
	"sync"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/config"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/types"
)

/*
Balance owns the wallet map and reservation ledger for the Desk event loop.
Snapshots replace the entire asset map; updates require exact next-sequence.
mu serializes wallet mutations against concurrent Frame/strategy reads.
*/
type Balance struct {
	status   atomic.Value
	api      *websocket.API
	mu       sync.RWMutex
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

	balance.mu.Lock()

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
		balance.mu.Unlock()
		balance.status.Store(types.READY)
		balance.Publish()

		return
	}

	if balance.sequence > 0 && incoming.Sequence != balance.sequence+1 {
		balance.mu.Unlock()
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
	balance.mu.Unlock()
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

	balance.mu.RLock()
	defer balance.mu.RUnlock()

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
		balance.mu.RLock()
		defer balance.mu.RUnlock()

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
	balance.mu.RLock()
	defer balance.mu.RUnlock()

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
	balance.mu.Lock()
	defer balance.mu.Unlock()

	balance.holdings[holding.Symbol] = holding
}

/*
DeleteHolding removes a pending lot that never filled.
*/
func (balance *Balance) DeleteHolding(symbol string) {
	balance.mu.Lock()
	defer balance.mu.Unlock()

	delete(balance.holdings, symbol)
}

/*
LookupHolding returns the live lot pointer for Desk fill/mark paths.
*/
func (balance *Balance) LookupHolding(symbol string) (*types.Holding, bool) {
	balance.mu.RLock()
	defer balance.mu.RUnlock()

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
