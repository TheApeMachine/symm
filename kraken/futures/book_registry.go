package futures

import (
	"sort"
	"sync"
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
)

/*
BookRegistry merges Kraken Futures book snapshots and deltas into L2 books.
*/
type BookRegistry struct {
	mu    sync.Mutex
	books map[string]*productBook
}

type productBook struct {
	bids map[float64]float64
	asks map[float64]float64
	seq  int
}

func NewBookRegistry() *BookRegistry {
	return &BookRegistry{
		books: make(map[string]*productBook),
	}
}

func (registry *BookRegistry) ApplySnapshot(snapshot BookSnapshot) *krakenmarket.BookUpdate {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	book := &productBook{
		bids: levelsToMap(snapshot.Bids),
		asks: levelsToMap(snapshot.Asks),
		seq:  snapshot.Seq,
	}

	registry.books[snapshot.ProductID] = book

	return registry.bookUpdate(snapshot.ProductID, book, snapshot.Timestamp, "snapshot")
}

func (registry *BookRegistry) ApplyDelta(delta BookDelta) (*krakenmarket.BookUpdate, bool) {
	registry.mu.Lock()
	defer registry.mu.Unlock()

	book, ok := registry.books[delta.ProductID]

	if !ok || delta.Seq <= book.seq {
		return nil, false
	}

	side := delta.Side

	switch side {
	case "buy":
		applyLevel(book.bids, delta.Price, delta.Qty)
	case "sell":
		applyLevel(book.asks, delta.Price, delta.Qty)
	default:
		return nil, false
	}

	book.seq = delta.Seq

	return registry.bookUpdate(delta.ProductID, book, delta.Timestamp, "update"), true
}

func (registry *BookRegistry) bookUpdate(
	productID string,
	book *productBook,
	timestampMillis int64,
	updateType string,
) *krakenmarket.BookUpdate {
	eventAt := time.UnixMilli(timestampMillis)

	if timestampMillis <= 0 {
		eventAt = time.Now()
	}

	return &krakenmarket.BookUpdate{
		Symbol:    productID,
		Bids:      mapToLevels(book.bids, true),
		Asks:      mapToLevels(book.asks, false),
		Timestamp: eventAt,
		Type:      updateType,
	}
}

func levelsToMap(levels []krakenmarket.BookLevel) map[float64]float64 {
	levelsMap := make(map[float64]float64, len(levels))

	for _, level := range levels {
		if level.Qty <= 0 {
			continue
		}

		levelsMap[level.Price] = level.Qty
	}

	return levelsMap
}

func applyLevel(levels map[float64]float64, price, qty float64) {
	if qty <= 0 {
		delete(levels, price)

		return
	}

	levels[price] = qty
}

func mapToLevels(levels map[float64]float64, descending bool) []krakenmarket.BookLevel {
	rows := make([]krakenmarket.BookLevel, 0, len(levels))

	for price, qty := range levels {
		if qty <= 0 {
			continue
		}

		rows = append(rows, krakenmarket.BookLevel{
			Price: price,
			Qty:   qty,
		})
	}

	sort.Slice(rows, func(left, right int) bool {
		if descending {
			return rows[left].Price > rows[right].Price
		}

		return rows[left].Price < rows[right].Price
	})

	return rows
}
