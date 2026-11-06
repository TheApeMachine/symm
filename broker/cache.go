package broker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
)

type Cache struct {
	*runtime.System
	mu        sync.RWMutex
	tierKey   string
	store     *store.Client
	transport network.Client
	price     *Price
	onSettled func(*Position)
	positions map[string]map[string]*Position
}

func NewCache(
	ctx context.Context,
	tierKey string,
	transport network.Client,
	price *Price,
	onSettled func(*Position),
	client *store.Client,
) *Cache {
	cache := &Cache{
		System:    runtime.NewSystem(ctx, "cache"),
		tierKey:   tierKey,
		store:     client,
		transport: transport,
		price:     price,
		onSettled: onSettled,
		positions: map[string]map[string]*Position{
			"open":   {},
			"closed": {},
		},
	}

	if cache.store == nil {
		return cache
	}

	for buf := range cache.store.Stream(cache.Context(), fmt.Sprintf("%s/open/", cache.tierKey)) {
		position := cache.Restore(buf)

		if position == nil {
			continue
		}

		// Open-prefix orphans that already sold flat belong under closed — never
		// re-arm them for a second exit (that is what painted INIT −4.4%).
		if position.Flat() {
			if position.ClosedAt == nil {
				closed := time.Now().UTC()
				position.ClosedAt = &closed
			}

			fee := decimal.Zero

			if position.price != nil {
				if rate := position.price.Fee(position.Symbol); rate != nil {
					fee = *rate
				}
			}

			position.feeNetPnL(position.Entry, position.Sold, position.ExitVWAP, &fee)
			cache.keepClosed(position)
			err := cache.move(position)

			if err != nil {
				errnie.Error(err)
			}

			continue
		}

		// Unfinished open-prefix settle: clear ClosedAt so mark can finish exit.
		position.ClosedAt = nil

		if position.Stoploss != nil {
			position.Stoploss.State = StoplossHealthy
		} else {
			pnlPct := 0.0

			if position.PnLPct != nil {
				pnlPct, _ = position.PnLPct.Float64()
			}

			position.Stoploss = NewStoploss(pnlPct)
		}

		cache.positions["open"][position.Symbol] = position
	}

	for buf := range cache.store.Stream(cache.Context(), fmt.Sprintf("%s/closed/", cache.tierKey)) {
		position := cache.Restore(buf)

		if position == nil || position.ClosedAt == nil {
			continue
		}

		cache.keepClosed(position)
	}

	return cache
}

func (cache *Cache) All(symbols ...string) []*Position {
	return append(cache.Open(symbols...), cache.Closed(symbols...)...)
}

/*
Open is filled live lots only — Qty/Entry set. Unfilled submits are not open.
*/
func (cache *Cache) Open(symbols ...string) []*Position {
	return cache.list("open", true, symbols...)
}

/*
Submitted is every lot under the open prefix, including awaiting first fill.
Apply uses this to match entry prints; Update must never see these.
*/
func (cache *Cache) Submitted(symbols ...string) []*Position {
	return cache.list("open", false, symbols...)
}

func (cache *Cache) Closed(symbols ...string) []*Position {
	return cache.list("closed", false, symbols...)
}

/*
Restore unmarshals a bare Position and rebinds live collaborators.
*/
func (cache *Cache) Restore(raw []byte) *Position {
	if len(raw) == 0 {
		return nil
	}

	var position *Position

	if err := sonic.Unmarshal(raw, &position); err != nil {
		errnie.Error(errnie.Err(
			errnie.Validation, "[broker.Cache.Restore] decode failed", err,
		))

		return nil
	}

	if position == nil || position.Symbol == "" {
		return nil
	}

	position.System = runtime.NewSystem(cache.Context(), "position")
	position.transport = cache.transport
	position.price = cache.price
	position.onSettled = cache.onSettled

	if position.TierKey == "" {
		position.TierKey = cache.tierKey
	}

	if position.Book == "" {
		position.Book = position.TierKey
	}

	// Legacy lots predate qty/entry/sold on the wire — replay fills through
	// Apply so Restore never folds outside the fill path.
	if position.Qty == nil && len(position.Fills) > 0 {
		fills := position.Fills
		position.Fills = nil
		closing := position.Closing
		closedAt := position.ClosedAt
		position.Closing = false
		position.ClosedAt = nil

		for _, fill := range fills {
			if fill == nil {
				continue
			}

			position.Apply(*fill)
		}

		position.Closing = closing

		if closedAt != nil {
			position.ClosedAt = closedAt
		}
	}

	return position
}

/*
keepClosed retains the latest settle when several closed ids share a symbol.
*/
func (cache *Cache) keepClosed(position *Position) {
	if position == nil {
		return
	}

	current := cache.positions["closed"][position.Symbol]

	if current != nil && current.ClosedAt != nil && position.ClosedAt != nil &&
		!position.ClosedAt.After(*current.ClosedAt) {
		return
	}

	cache.positions["closed"][position.Symbol] = position
}

func (cache *Cache) Has(symbol string) bool {
	cache.mu.Lock()
	defer cache.mu.Unlock()

	// Still under open (including ClosedAt claimed while settle Sends) occupies
	// the symbol. Remove is the only path into closed.
	return cache.positions["open"][symbol] != nil
}

func (cache *Cache) Add(symbol string, position *Position, open bool) bool {
	if symbol == "" || position == nil {
		return true
	}

	state := "closed"

	if open {
		state = "open"
	}

	cache.mu.Lock()

	if existing, loaded := cache.positions[state][symbol]; loaded {
		if !(open && existing != nil && existing.ClosedAt != nil) {
			cache.mu.Unlock()
			return true
		}
	}

	cache.positions[state][symbol] = position
	cache.mu.Unlock()

	return false
}

/*
Save writes the full lot (orders, fills, mark, pnl, stoploss) to S3.

Lots already under closed/ (or settled and no longer open) update the closed
object and retire any open/ orphan — exit fills after Remove must not recreate
an open key with a second mark pass.
*/
func (cache *Cache) Save(position *Position) error {
	if position == nil || position.ID == "" {
		return nil
	}

	body, err := sonic.Marshal(position)

	if err != nil {
		return errnie.Error(err)
	}

	cache.mu.RLock()
	inClosed := cache.positions["closed"][position.Symbol] == position
	inOpen := cache.positions["open"][position.Symbol] == position
	cache.mu.RUnlock()

	if inClosed || (position.ClosedAt != nil && !inOpen) {
		closed := fmt.Sprintf("%s/closed/%s/%s", cache.tierKey, position.Symbol, position.ID)
		open := fmt.Sprintf("%s/open/%s/%s", cache.tierKey, position.Symbol, position.ID)

		if _, err := cache.store.WriteAtPrefix(closed, body); err != nil {
			return errnie.Error(err)
		}

		if err := cache.store.DeletePrefix(open); err != nil {
			return errnie.Error(err)
		}

		return nil
	}

	if _, err := cache.store.WriteAtPrefix(fmt.Sprintf(
		"%s/open/%s/%s", cache.tierKey, position.Symbol, position.ID,
	), body); err != nil {
		return errnie.Error(err)
	}

	return nil
}

func (cache *Cache) Remove(symbol string) bool {
	cache.mu.Lock()

	loaded, ok := cache.positions["open"][symbol]

	if !ok {
		cache.mu.Unlock()
		return false
	}

	delete(cache.positions["open"], symbol)
	cache.positions["closed"][symbol] = loaded
	cache.mu.Unlock()

	if err := cache.move(loaded); err != nil {
		errnie.Error(err)
	}

	return true
}

func (cache *Cache) Drop(symbol string) {
	if symbol == "" {
		return
	}

	cache.mu.Lock()
	delete(cache.positions["open"], symbol)
	delete(cache.positions["closed"], symbol)
	cache.mu.Unlock()

	// Symbol-scoped prefixes clear every orphan ID under the pair, not only
	// the lot currently held in memory.
	err := cache.store.DeletePrefix(fmt.Sprintf("%s/open/%s/", cache.tierKey, symbol))
	
	if err != nil {
		errnie.Error(err)
	}
	
	err = cache.store.DeletePrefix(fmt.Sprintf("%s/closed/%s/", cache.tierKey, symbol))
	
	if err != nil {
		errnie.Error(err)
	}
}

func (cache *Cache) move(position *Position) error {
	if position == nil || position.ID == "" {
		return nil
	}

	body, err := sonic.Marshal(position)

	if err != nil {
		return errnie.Error(err)
	}

	closed := fmt.Sprintf("%s/closed/%s/%s", cache.tierKey, position.Symbol, position.ID)
	open := fmt.Sprintf("%s/open/%s/%s", cache.tierKey, position.Symbol, position.ID)

	if _, err := cache.store.WriteAtPrefix(closed, body); err != nil {
		return err
	}

	if err := cache.store.DeletePrefix(open); err != nil {
		return errnie.Error(errnie.Err(
			errnie.IO,
			"[broker.Cache.move] failed to retire open lot",
			err,
		))
	}

	return nil
}

func (cache *Cache) list(state string, filledOnly bool, symbols ...string) []*Position {
	positions := make([]*Position, 0)

	cache.mu.RLock()
	defer cache.mu.RUnlock()

	appendIf := func(position *Position) {
		if position == nil {
			return
		}

		if state == "open" && position.ClosedAt != nil {
			return
		}

		if filledOnly &&
			(position.Qty == nil || position.Qty.Sign() <= 0 ||
				position.Entry == nil || position.Entry.Sign() <= 0) {
			return
		}

		positions = append(positions, position)
	}

	if len(symbols) == 0 {
		for _, position := range cache.positions[state] {
			appendIf(position)
		}

		return positions
	}

	for _, symbol := range symbols {
		appendIf(cache.positions[state][symbol])
	}

	return positions
}
