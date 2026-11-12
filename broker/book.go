package broker

import (
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
)

/*
Book is the historical alias signals and boot still use for BookManager.
*/
type Book = BookManager

/*
bookLane is one symbol's book plus its readiness flags. Update/Book take this
lock so an L3 write on BTC does not block readers of ETH.
*/
type bookLane struct {
	mu      sync.RWMutex
	book    *market.Book
	pending bool
	stale   bool
}

/*
BookManager holds per-symbol market books. Venue frames arrive as market.Level3
(converted at the edge); this type never imports a venue package.
*/
type BookManager struct {
	mu        sync.RWMutex
	lanes     map[string]*bookLane
	pendingN  atomic.Int64
	seeded    chan struct{}
	onInvalid func([]string)
}

/*
NewBookManager exists so boot can register Expect/Wait before the first L3
frame; pending counts and the seeded signal live here rather than in Price.
*/
func NewBookManager() *BookManager {
	return &BookManager{
		lanes:  make(map[string]*bookLane),
		seeded: make(chan struct{}, 1),
	}
}

/*
OnInvalid registers recovery after a checksum miss or crossed book.
*/
func (manager *BookManager) OnInvalid(fn func([]string)) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.onInvalid = fn
}

/*
Expect marks each subscribed symbol pending so startup can block: Allocate and
TouchBid must not run on an empty or partial book after subscribe.
*/
func (manager *BookManager) Expect(symbols []string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.lanes = make(map[string]*bookLane, len(symbols))
	pending := int64(0)

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		if _, exists := manager.lanes[symbol]; exists {
			continue
		}

		manager.lanes[symbol] = &bookLane{pending: true}
		pending++
	}

	manager.pendingN.Store(pending)
}

/*
Wait is the boot gate after SubscribeBooks — strategy should not enter until
every Expect'd symbol has applied its first snapshot or sizing lies.
*/
func (manager *BookManager) Wait(timeout time.Duration) error {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()

	for {
		if len(manager.Pending()) == 0 {
			return nil
		}

		select {
		case <-deadline.C:
			pending := manager.Pending()

			return errnie.Err(
				errnie.NotFound,
				fmt.Sprintf("book: %d level3 books not seeded within %s: %s",
					len(pending), timeout, strings.Join(pending, ",")),
				nil,
			)
		case <-manager.seeded:
		}
	}
}

/*
Pending feeds Wait timeouts and error text listing which symbols never seeded.
*/
func (manager *BookManager) Pending() []string {
	manager.mu.RLock()
	type pendingLane struct {
		symbol string
		lane   *bookLane
	}

	snapshot := make([]pendingLane, 0, len(manager.lanes))

	for symbol, lane := range manager.lanes {
		snapshot = append(snapshot, pendingLane{symbol: symbol, lane: lane})
	}

	manager.mu.RUnlock()

	out := make([]string, 0)

	for _, item := range snapshot {
		item.lane.mu.RLock()
		pending := item.lane.pending
		item.lane.mu.RUnlock()

		if pending {
			out = append(out, item.symbol)
		}
	}

	slices.Sort(out)
	return out
}

/*
lookup avoids inventing lanes on read: a missing key must stay "not seeded",
not flip to an empty book that passed BookIntact checks.
*/
func (manager *BookManager) lookup(symbol string) *bookLane {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	return manager.lanes[symbol]
}

/*
lane creates on write because resubscribe and invalidation can deliver deltas
before boot called Expect for that symbol.
*/
func (manager *BookManager) lane(symbol string) *bookLane {
	if lane := manager.lookup(symbol); lane != nil {
		return lane
	}

	manager.mu.Lock()
	defer manager.mu.Unlock()

	if lane := manager.lanes[symbol]; lane != nil {
		return lane
	}

	lane := &bookLane{}
	manager.lanes[symbol] = lane

	return lane
}

/*
Update applies L3 under per-symbol locks so one checksum failure does not stall
the whole universe; invalid symbols trigger instrument resubscribe.
*/
func (manager *BookManager) Update(level3 *market.Level3) {
	if level3 == nil {
		return
	}

	var invalid []string

	for _, data := range level3.Data {
		if data == nil || data.Symbol == "" {
			continue
		}

		lane := manager.lane(data.Symbol)
		lane.mu.Lock()

		book := lane.book

		// Snapshots are full state — replace so ghosts from a prior
		// stream (or missed deletes) cannot stick around.
		if book == nil || level3.Type == "snapshot" {
			book = market.NewBook()
			book.Name = data.Symbol
			// Kraken does not delete levels that fall out of subscribed
			// depth; EnforceDepth must match the subscription.
			book.MaxDepth = system.Cfg.Market.Level3.Depth
			lane.book = book
		}

		for _, order := range data.Bids {
			manager.applyOrder(book, market.SideBid, order)
		}

		for _, order := range data.Asks {
			manager.applyOrder(book, market.SideAsk, order)
		}

		if manager.diverged(book, data.Checksum) {
			invalid = append(invalid, data.Symbol)
			lane.stale = true
			manager.clearPending(lane)
			lane.book = nil
			lane.mu.Unlock()
			continue
		}

		manager.clearPending(lane)
		lane.stale = false
		lane.mu.Unlock()
	}

	if manager.pendingN.Load() == 0 {
		select {
		case manager.seeded <- struct{}{}:
		default:
		}
	}

	manager.mu.RLock()
	onInvalid := manager.onInvalid
	manager.mu.RUnlock()

	if len(invalid) > 0 && onInvalid != nil {
		onInvalid(invalid)
	}
}

/*
clearPending bumps pendingN and pings seeded so Wait can wake without scanning
all lanes on every delta frame.
*/
func (manager *BookManager) clearPending(lane *bookLane) {
	if !lane.pending {
		return
	}

	lane.pending = false
	manager.pendingN.Add(-1)
}

/*
diverged forces invalidation instead of serving a book that fails Kraken's
checksum or shows an impossible touch — continuing would poison Allocate and marks.
*/
func (manager *BookManager) diverged(book *market.Book, checksum int) bool {
	if book == nil {
		return true
	}

	bid, ask := book.BestBid(), book.BestAsk()

	if bid != nil && ask != nil && bid.Price != nil && ask.Price != nil &&
		bid.Price.Cmp(*ask.Price) >= 0 {
		return true
	}

	if checksum == 0 {
		return false
	}

	result := book.L3Checksum(strconv.Itoa(checksum))
	return result == nil || !result.Match
}

/*
applyOrder maps Kraken L3 rows into nomagique deltas; delete events keep stale
qty so the book actually drops levels the venue only flags via event=delete.
*/
func (manager *BookManager) applyOrder(
	book *market.Book,
	side market.SideKind,
	order *market.Level3Order,
) {
	if order == nil || order.LimitPrice == nil {
		return
	}

	// Kraken delete frames still carry the last positive order_qty; removal
	// is signaled only by event=delete. Force qty 0 so the book drops it.
	qty := order.OrderQty

	if order.Event == "delete" {
		zero := decimal.Zero
		qty = &zero
	}

	if qty == nil {
		return
	}

	book.Update(&market.Delta{
		Direction: side,
		ID:        order.OrderID,
		Price:     order.LimitPrice,
		Quantity:  qty,
		Timestamp: order.Timestamp,
	})
}

/*
Get exposes the book pointer for tests; production marks use Book so depth walks
do not hold the lane lock across Price.Allocate.
*/
func (manager *BookManager) Get(symbol string) *market.Book {
	lane := manager.lookup(symbol)
	if lane == nil {
		return nil
	}

	lane.mu.RLock()
	defer lane.mu.RUnlock()

	return lane.book
}

/*
Book is the hot-path reader: callback releases the lane lock before ask/bid walks
so L3 writers on other symbols are not blocked for the whole Allocate.
*/
func (manager *BookManager) Book(symbol string, read func(*market.Book)) {
	lane := manager.lookup(symbol)
	if lane == nil {
		return
	}

	lane.mu.RLock()
	defer lane.mu.RUnlock()

	if lane.pending || lane.stale || lane.book == nil {
		return
	}

	read(lane.book)
}

/*
BlockReason turns pending/stale/nil into Allocate's "why" string instead of a
generic book failure.
*/
func (manager *BookManager) BlockReason(symbol string) string {
	if symbol == "" {
		return "symbol is empty"
	}

	lane := manager.lookup(symbol)
	if lane == nil {
		return "level3 book not seeded"
	}

	lane.mu.RLock()
	defer lane.mu.RUnlock()

	if lane.pending {
		return "level3 book still pending snapshot"
	}

	if lane.stale {
		return "level3 book is stale"
	}

	if lane.book == nil {
		return "level3 book not seeded"
	}

	return ""
}

/*
MarkPaused is true while level3 is reconnecting — marks may use ticker touch but
must not arm stoploss or size entries against a nil book.
*/
func (manager *BookManager) MarkPaused(symbol string) bool {
	switch manager.BlockReason(symbol) {
	case "level3 book is stale", "level3 book still pending snapshot":
		return true
	default:
		return false
	}
}

/*
StaleSymbol gates Desk.BookStale and BookIntact so disconnects stop entries before
marks use a book the socket no longer feeds.
*/
func (manager *BookManager) StaleSymbol(symbol string) bool {
	lane := manager.lookup(symbol)
	if lane == nil {
		return true
	}

	lane.mu.RLock()
	defer lane.mu.RUnlock()

	return lane.stale || lane.book == nil
}

/*
Stale is wired from Instrument OnDisconnect so marks and sizing halt until
ResubscribeLevel3 delivers a fresh snapshot.
*/
func (manager *BookManager) Stale(symbols []string) {
	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		lane := manager.lane(symbol)
		lane.mu.Lock()
		lane.stale = true
		lane.book = nil
		lane.mu.Unlock()
	}
}

/*
All iterates seeded books for tests and desk helpers that need a universe sweep
without knowing the symbol list ahead of time.
*/
func (manager *BookManager) All() iter.Seq[*market.Book] {
	return func(yield func(*market.Book) bool) {
		manager.mu.RLock()
		lanes := make([]*bookLane, 0, len(manager.lanes))

		for _, lane := range manager.lanes {
			lanes = append(lanes, lane)
		}

		manager.mu.RUnlock()

		for _, lane := range lanes {
			lane.mu.RLock()
			book := lane.book
			lane.mu.RUnlock()

			if book == nil {
				continue
			}

			if !yield(book) {
				return
			}
		}
	}
}
