package broker

import (
	"fmt"
	"iter"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/shopspring/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
	"github.com/theapemachine/symm/system"
)

// Book is the historical name signals/boot still use for BookManager.
type Book = BookManager

/*
BookManager holds per-symbol market books. Venue frames arrive as market.Level3
(converted at the edge); this type never imports a venue package.
*/
type BookManager struct {
	mu        sync.RWMutex
	books     map[string]*market.Book
	pending   map[string]struct{}
	stale     map[string]struct{}
	seeded    chan struct{}
	onInvalid func([]string)
}

func NewBookManager() *BookManager {
	return &BookManager{
		books:   make(map[string]*market.Book),
		pending: make(map[string]struct{}),
		stale:   make(map[string]struct{}),
		seeded:  make(chan struct{}, 1),
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

func (manager *BookManager) Expect(symbols []string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	manager.pending = make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		if symbol != "" {
			manager.pending[symbol] = struct{}{}
		}
	}
}

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

func (manager *BookManager) Pending() []string {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	out := make([]string, 0, len(manager.pending))

	for symbol := range manager.pending {
		out = append(out, symbol)
	}

	slices.Sort(out)
	return out
}

func (manager *BookManager) Update(level3 *market.Level3) {
	if level3 == nil {
		return
	}

	manager.mu.Lock()

	var invalid []string

	for _, data := range level3.Data {
		if data == nil || data.Symbol == "" {
			continue
		}

		book := manager.books[data.Symbol]

		// Snapshots are full state — replace so ghosts from a prior
		// stream (or missed deletes) cannot stick around.
		if book == nil || level3.Type == "snapshot" {
			book = market.NewBook()
			book.Name = data.Symbol
			// Kraken does not delete levels that fall out of subscribed
			// depth; EnforceDepth must match the subscription.
			book.MaxDepth = system.Cfg.Market.Level3.Depth
			manager.books[data.Symbol] = book
		}

		for _, order := range data.Bids {
			manager.applyOrder(book, market.SideBid, order)
		}

		for _, order := range data.Asks {
			manager.applyOrder(book, market.SideAsk, order)
		}

		if manager.diverged(book, data.Checksum) {
			invalid = append(invalid, data.Symbol)
			manager.stale[data.Symbol] = struct{}{}
			delete(manager.books, data.Symbol)
			continue
		}

		delete(manager.pending, data.Symbol)
		delete(manager.stale, data.Symbol)
	}

	if len(manager.pending) == 0 {
		select {
		case manager.seeded <- struct{}{}:
		default:
		}
	}

	onInvalid := manager.onInvalid
	manager.mu.Unlock()

	if len(invalid) > 0 && onInvalid != nil {
		onInvalid(invalid)
	}
}

/*
diverged is true when the venue checksum misses or the book is crossed.
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

func (manager *BookManager) Get(symbol string) *market.Book {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	return manager.books[symbol]
}

func (manager *BookManager) Book(symbol string, read func(*market.Book)) {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	if _, pending := manager.pending[symbol]; pending {
		return
	}

	if _, stale := manager.stale[symbol]; stale {
		return
	}

	if book := manager.books[symbol]; book != nil {
		read(book)
	}
}

func (manager *BookManager) BlockReason(symbol string) string {
	if symbol == "" {
		return "symbol is empty"
	}

	manager.mu.RLock()
	defer manager.mu.RUnlock()

	if _, pending := manager.pending[symbol]; pending {
		return "level3 book still pending snapshot"
	}

	if _, stale := manager.stale[symbol]; stale {
		return "level3 book is stale"
	}

	if manager.books[symbol] == nil {
		return "level3 book not seeded"
	}

	return ""
}

func (manager *BookManager) StaleSymbol(symbol string) bool {
	manager.mu.RLock()
	defer manager.mu.RUnlock()

	_, stale := manager.stale[symbol]
	return stale || manager.books[symbol] == nil
}

func (manager *BookManager) Stale(symbols []string) {
	manager.mu.Lock()
	defer manager.mu.Unlock()

	for _, symbol := range symbols {
		if symbol == "" {
			continue
		}

		manager.stale[symbol] = struct{}{}
		delete(manager.books, symbol)
	}
}

func (manager *BookManager) All() iter.Seq[*market.Book] {
	return func(yield func(*market.Book) bool) {
		manager.mu.RLock()
		defer manager.mu.RUnlock()

		for _, book := range manager.books {
			if !yield(book) {
				return
			}
		}
	}
}
