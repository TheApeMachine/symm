package broker

import (
	"iter"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
Inventory owns open and closed lots. It locks its own map; Sync receives quote,
wallet rows, and a reserved-qty lookup so it never reaches back into Balance.
*/
type Inventory struct {
	mu   sync.RWMutex
	lots map[string]*types.Holding
}

/*
NewInventory constructs empty lot storage for a Balance to compose.
*/
func NewInventory() *Inventory {
	return &Inventory{lots: make(map[string]*types.Holding)}
}

/*
Holdings yields open lots as value copies and releases the lock before yield so
callers may Update without re-entering the mutex.
*/
func (inventory *Inventory) Holdings() iter.Seq[types.Holding] {
	return func(yield func(types.Holding) bool) {
		inventory.mu.RLock()
		copies := make([]types.Holding, 0, len(inventory.lots))

		for symbol, holding := range inventory.lots {
			if symbol != holding.Symbol {
				continue
			}

			if holding.Status == types.CLOSED {
				continue
			}

			copies = append(copies, *holding)
		}

		inventory.mu.RUnlock()

		for _, holding := range copies {
			if !yield(holding) {
				return
			}
		}
	}
}

/*
Holding returns a value copy of an open lot.
*/
func (inventory *Inventory) Holding(symbol string) (types.Holding, error) {
	inventory.mu.RLock()
	defer inventory.mu.RUnlock()

	holding, ok := inventory.lots[symbol]

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
StoreHolding writes a lot on the Desk enter / adopt path.
*/
func (inventory *Inventory) StoreHolding(holding *types.Holding) {
	inventory.mu.Lock()
	defer inventory.mu.Unlock()

	inventory.lots[holding.Symbol] = holding
}

/*
DeleteHolding removes a pending lot that never filled.
*/
func (inventory *Inventory) DeleteHolding(symbol string) {
	inventory.mu.Lock()
	defer inventory.mu.Unlock()

	delete(inventory.lots, symbol)
}

/*
Update runs fn against the live lot under Inventory.mu so fill and mark paths
cannot race Sync.
*/
func (inventory *Inventory) Update(
	symbol string,
	fn func(*types.Holding) error,
) error {
	inventory.mu.Lock()
	defer inventory.mu.Unlock()

	holding, ok := inventory.lots[symbol]

	if !ok {
		return errnie.Error(errnie.Err(
			errnie.NotFound,
			"holding not found for "+symbol,
			nil,
		))
	}

	return fn(holding)
}

/*
Lots yields every retained holding, including closed lots for the audit rail.
*/
func (inventory *Inventory) Lots() iter.Seq[types.Holding] {
	return func(yield func(types.Holding) bool) {
		inventory.mu.RLock()
		defer inventory.mu.RUnlock()

		for symbol, holding := range inventory.lots {
			if symbol != holding.Symbol {
				continue
			}

			if !yield(*holding) {
				return
			}
		}
	}
}

/*
Sync materializes open holdings from non-quote wallet balances and closes lots
whose exchange qty has gone to zero. reserved supplies live sell claims per asset.
*/
func (inventory *Inventory) Sync(
	quote string,
	data map[string]*kraken.BalanceData,
	reserved func(asset string) *decimal.Decimal,
) {
	inventory.mu.Lock()
	defer inventory.mu.Unlock()

	seen := make(map[string]struct{})

	for asset, row := range data {
		if asset == "" || asset == quote {
			continue
		}

		if row.Balance == nil || row.Balance.Sign() <= 0 {
			inventory.close(quote, asset)
			continue
		}

		symbol := asset + "/" + quote
		seen[symbol] = struct{}{}
		inventory.upsert(symbol, asset, row.Balance, reserved(asset))
	}

	for _, holding := range inventory.lots {
		if holding.Status != types.OPEN {
			continue
		}

		if _, ok := seen[holding.Symbol]; ok {
			continue
		}

		if holding.Asset == "" {
			continue
		}

		// Desk-traded lots must not flatten when a snapshot omits the asset —
		// wait for an explicit zero-qty row.
		if holding.EntryPrice != nil ||
			holding.Qty != nil && holding.Qty.Sign() > 0 {
			continue
		}

		status, err := types.Transition(holding.Status, types.CLOSED)

		if err != nil {
			errnie.Error(err)

			continue
		}

		holding.Status = status
		holding.Qty = decimal.NewFromInt64(0)
		holding.SellableQty = decimal.NewFromInt64(0)
	}
}

/*
upsert opens or refreshes a wallet-backed holding from exchange qty.
Caller must already hold Inventory.mu.
*/
func (inventory *Inventory) upsert(
	symbol, asset string,
	qty, reserved *decimal.Decimal,
) {
	sellable := qty.Copy().Sub(reserved)

	if holding, ok := inventory.lots[symbol]; ok {
		holding.Qty = qty.Copy()
		holding.Asset = asset
		holding.SellableQty = sellable

		if holding.Status == types.CLOSED || holding.Status == types.CANCELED {
			status, err := types.Transition(holding.Status, types.OPEN)

			if err != nil {
				errnie.Error(err)

				return
			}

			holding.Status = status
		}

		return
	}

	inventory.lots[symbol] = &types.Holding{
		Symbol:      symbol,
		Asset:       asset,
		Qty:         qty.Copy(),
		SellableQty: sellable,
		Status:      types.OPEN,
	}
}

/*
close marks a wallet-backed lot closed when exchange qty is zero.
Caller must already hold Inventory.mu.
*/
func (inventory *Inventory) close(quote, asset string) {
	symbol := asset + "/" + quote
	holding, ok := inventory.lots[symbol]

	if !ok {
		return
	}

	status, err := types.Transition(holding.Status, types.CLOSED)

	if err != nil {
		errnie.Error(err)

		return
	}

	holding.Status = status
	holding.Qty = decimal.NewFromInt64(0)
	holding.SellableQty = decimal.NewFromInt64(0)
}
