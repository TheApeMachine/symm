package broker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/runtime"
	"github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/network"
)

type Cache struct {
	*runtime.System
	tierKey   string
	store     *store.Client
	transport network.Client
	price     *Price
	onSettled func(*Position)
	open      *sync.Map
	closed    *sync.Map
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
		open:      &sync.Map{},
		closed:    &sync.Map{},
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
		// Keep wire PnL; do not re-price with today's fee table.
		if position.Flat() {
			if position.ClosedAt == nil {
				closed := time.Now().UTC()
				position.ClosedAt = &closed
			}

			if position.PnL == nil || position.PnLPct == nil {
				errnie.Error(errnie.Err(
					errnie.UnprocessableContent,
					"[broker.Cache] flat open orphan missing settle pnl for "+position.Symbol,
					nil,
				))
			}

			cache.keepClosed(position)
			err := cache.move(position)

			if err != nil {
				errnie.Error(err)
			}

			continue
		}

		// Unfinished open-prefix settle: clear ClosedAt so mark can finish
		// exit, but keep Closing/ExitTick so a late fill and resend work.
		position.ClosedAt = nil

		if position.Stoploss == nil {
			pnlPct := 0.0

			if position.PnLPct != nil {
				pnlPct, _ = position.PnLPct.Float64()
			}

			position.Stoploss = NewStoploss(pnlPct)
		}

		cache.open.Store(position.Symbol, position)
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
	return cache.list(cache.open, true, true, symbols...)
}

/*
Submitted is every lot under the open prefix, including awaiting first fill.
Apply uses this to match entry prints; Update must never see these.
*/
func (cache *Cache) Submitted(symbols ...string) []*Position {
	return cache.list(cache.open, true, false, symbols...)
}

func (cache *Cache) Closed(symbols ...string) []*Position {
	return cache.list(cache.closed, false, false, symbols...)
}

/*
lotBlob is the S3 / DuckDB / UI envelope around a Position.
*/
type lotBlob struct {
	Epoch     uint64    `json:"epoch"`
	Tick      uint64    `json:"tick"`
	Timestamp uint64    `json:"timestamp"`
	Position  *Position `json:"position"`
}

func encodeLot(position *Position) ([]byte, error) {
	blob := lotBlob{
		Epoch:    position.PolicyEpoch,
		Tick:     position.EntryTick,
		Position: position,
	}

	if position.ExitTick > 0 {
		blob.Tick = position.ExitTick
	}

	switch {
	case position.ClosedAt != nil:
		blob.Timestamp = uint64(position.ClosedAt.UnixNano())
	case position.UpdatedAt != nil:
		blob.Timestamp = uint64(position.UpdatedAt.UnixNano())
	default:
		blob.Timestamp = uint64(position.OpenedAt.UnixNano())
	}

	return sonic.Marshal(blob)
}

/*
Restore unmarshals a lot blob (or legacy bare Position) and rebinds collaborators.
*/
func (cache *Cache) Restore(raw []byte) *Position {
	if len(raw) == 0 {
		return nil
	}

	var position *Position
	var wrapped lotBlob

	if err := sonic.Unmarshal(raw, &wrapped); err == nil && wrapped.Position != nil {
		position = wrapped.Position
	} else if err := sonic.Unmarshal(raw, &position); err != nil {
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

	for {
		raw, loaded := cache.closed.Load(position.Symbol)

		if !loaded {
			if _, raced := cache.closed.LoadOrStore(position.Symbol, position); raced {
				continue
			}

			return
		}

		current := raw.(*Position)

		if current != nil && current.ClosedAt != nil && position.ClosedAt != nil &&
			!position.ClosedAt.After(*current.ClosedAt) {
			return
		}

		if cache.closed.CompareAndSwap(position.Symbol, current, position) {
			return
		}
	}
}

func (cache *Cache) Has(symbol string) bool {
	// Still under open (including ClosedAt claimed while settle Sends) occupies
	// the symbol. Remove is the only path into closed.
	_, ok := cache.open.Load(symbol)
	return ok
}

func (cache *Cache) Add(symbol string, position *Position, open bool) bool {
	if symbol == "" || position == nil {
		return true
	}

	lane := cache.closed

	if open {
		lane = cache.open
	}

	if existing, loaded := lane.Load(symbol); loaded {
		pos := existing.(*Position)

		if !(open && pos != nil && pos.ClosedAt != nil) {
			return true
		}
	}

	lane.Store(symbol, position)
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

	position.mu.Lock()
	body, err := encodeLot(position)
	symbol := position.Symbol
	id := position.ID
	closedAt := position.ClosedAt
	position.mu.Unlock()

	if err != nil {
		return errnie.Error(err)
	}

	return cache.writeLot(position, symbol, id, closedAt != nil, body)
}

/*
SaveAsync runs Save off the mark path so encode + upload never stall Tier.Update.
*/
func (cache *Cache) SaveAsync(position *Position) {
	if position == nil || position.ID == "" || cache.store == nil {
		return
	}

	go func() {
		if err := cache.Save(position); err != nil {
			errnie.Error(err)
		}
	}()
}

func (cache *Cache) writeLot(
	position *Position,
	symbol, id string,
	closed bool,
	body []byte,
) error {
	inClosed := cache.hasLot(cache.closed, symbol, position)
	inOpen := cache.hasLot(cache.open, symbol, position)

	if inClosed || (closed && !inOpen) {
		closedKey := fmt.Sprintf("%s/closed/%s/%s", cache.tierKey, symbol, id)
		openKey := fmt.Sprintf("%s/open/%s/%s", cache.tierKey, symbol, id)

		if _, err := cache.store.WriteAtPrefix(closedKey, body); err != nil {
			return errnie.Error(err)
		}

		if err := cache.store.DeletePrefix(openKey); err != nil {
			return errnie.Error(err)
		}

		return nil
	}

	if _, err := cache.store.WriteAtPrefix(fmt.Sprintf(
		"%s/open/%s/%s", cache.tierKey, symbol, id,
	), body); err != nil {
		return errnie.Error(err)
	}

	return nil
}

func (cache *Cache) hasLot(lane *sync.Map, symbol string, position *Position) bool {
	raw, ok := lane.Load(symbol)
	return ok && raw.(*Position) == position
}

func (cache *Cache) Remove(symbol string) bool {
	raw, ok := cache.open.LoadAndDelete(symbol)

	if !ok {
		return false
	}

	loaded := raw.(*Position)
	cache.closed.Store(symbol, loaded)

	if err := cache.move(loaded); err != nil {
		errnie.Error(err)
	}

	return true
}

func (cache *Cache) Drop(symbol string) {
	if symbol == "" {
		return
	}

	cache.open.Delete(symbol)

	// Only the failed open lot — closed history must survive a reject.
	if cache.store == nil {
		return
	}

	if err := cache.store.DeletePrefix(fmt.Sprintf("%s/open/%s/", cache.tierKey, symbol)); err != nil {
		errnie.Error(err)
	}
}

func (cache *Cache) move(position *Position) error {
	if position == nil || position.ID == "" || cache.store == nil {
		return nil
	}

	body, err := encodeLot(position)

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

func (cache *Cache) list(
	lane *sync.Map,
	skipClosedAt, filledOnly bool,
	symbols ...string,
) []*Position {
	positions := make([]*Position, 0)

	appendIf := func(position *Position) {
		if position == nil {
			return
		}

		if skipClosedAt && position.ClosedAt != nil {
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
		lane.Range(func(_, value any) bool {
			appendIf(value.(*Position))
			return true
		})

		return positions
	}

	for _, symbol := range symbols {
		if raw, ok := lane.Load(symbol); ok {
			appendIf(raw.(*Position))
		}
	}

	return positions
}
