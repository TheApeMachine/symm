package broker

import (
	"fmt"
	"iter"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/market"
)

// Book is the historical name signals/boot still use for BookManager.
type Book = BookManager

/*
BookManager holds per-symbol market books. Venue frames arrive as market.Level3
(converted at the edge); this type never imports a venue package.
*/
type BookManager struct {
	mu      sync.RWMutex
	books   map[string]*market.Book
	pending map[string]struct{}
	stale   map[string]struct{}
	seeded  chan struct{}
}

func NewBookManager() *BookManager {
	return &BookManager{
		books:   make(map[string]*market.Book),
		pending: make(map[string]struct{}),
		stale:   make(map[string]struct{}),
		seeded:  make(chan struct{}, 1),
	}
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
	defer manager.mu.Unlock()

	for _, data := range level3.Data {
		if data == nil || data.Symbol == "" {
			continue
		}

		book := manager.books[data.Symbol]

		if book == nil {
			book = market.NewBook()
			book.Name = data.Symbol
			manager.books[data.Symbol] = book
		}

		for _, order := range data.Bids {
			applyOrder(book, market.SideBid, order)
		}

		for _, order := range data.Asks {
			applyOrder(book, market.SideAsk, order)
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
}

func applyOrder(book *market.Book, side market.SideKind, order *market.Level3Order) {
	if order == nil || order.LimitPrice == nil || order.OrderQty == nil {
		return
	}

	book.Update(&market.Delta{
		Direction: side,
		ID:        order.OrderID,
		Price:     order.LimitPrice,
		Quantity:  order.OrderQty,
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
