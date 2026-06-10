package market

import (
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
)

const krakenChecksumLevels = 10

/*
BookStore maintains canonical L2 books and validates Kraken checksums.
Each symbol book is an atomic pointer updated with compare-and-swap.
*/
type BookStore struct {
	books          sync.Map
	depth          int
	checksumLevels int
}

func NewBookStore(depth int) *BookStore {
	if depth <= 0 {
		depth = krakenChecksumLevels
	}

	return &BookStore{
		depth:          depth,
		checksumLevels: krakenChecksumLevels,
	}
}

func (store *BookStore) Apply(update *BookUpdate) error {
	if store == nil || update == nil || update.Symbol == "" {
		return nil
	}

	bookSlot := store.bookSlot(update.Symbol)

	for {
		current := bookSlot.Load()

		merged, mergeErr := store.mergeUpdate(current, update)

		if mergeErr != nil {
			return mergeErr
		}

		if verifyErr := store.verify(update.Symbol, merged); verifyErr != nil {
			if update.Type == "snapshot" {
				bookSlot.Store(nil)
			}

			return verifyErr
		}

		if bookSlot.CompareAndSwap(current, merged) {
			return nil
		}
	}
}

func (store *BookStore) mergeUpdate(current *BookUpdate, update *BookUpdate) (*BookUpdate, error) {
	switch update.Type {
	case "snapshot":
		merged := cloneBookUpdate(update)
		store.truncateBook(merged)

		return merged, nil
	case "update":
		if current == nil {
			return nil, fmt.Errorf("bookstore: %s update before snapshot", update.Symbol)
		}

		merged := mergeBookUpdate(current, update)
		store.truncateBook(merged)

		return merged, nil
	default:
		if current == nil {
			merged := cloneBookUpdate(update)
			store.truncateBook(merged)

			return merged, nil
		}

		merged := mergeBookUpdate(current, update)
		store.truncateBook(merged)

		return merged, nil
	}
}

func (store *BookStore) truncateBook(book *BookUpdate) {
	if book == nil {
		return
	}

	book.Bids = truncateSide(book.Bids, store.depth)
	book.Asks = truncateSide(book.Asks, store.depth)
}

func truncateSide(levels []BookLevel, depth int) []BookLevel {
	if depth <= 0 || len(levels) <= depth {
		return levels
	}

	return levels[:depth]
}

func (store *BookStore) Snapshot(symbol string) (*BookUpdate, bool) {
	if store == nil {
		return nil, false
	}

	raw, ok := store.books.Load(symbol)

	if !ok {
		return nil, false
	}

	bookSlot := raw.(*atomic.Pointer[BookUpdate])
	book := bookSlot.Load()

	if book == nil {
		return nil, false
	}

	return cloneBookUpdate(book), true
}

func (store *BookStore) VWAP(
	symbol string,
	side string,
	quantity float64,
) (float64, float64, error) {
	book, ok := store.Snapshot(symbol)

	if !ok || book == nil {
		return 0, 0, fmt.Errorf("bookstore: no book for %s", symbol)
	}

	levels := book.Asks

	if side == "sell" {
		levels = book.Bids
	}

	remaining := quantity
	cost := 0.0
	filled := 0.0

	for _, level := range levels {
		if remaining <= 0 {
			break
		}

		take := level.Qty

		if take > remaining {
			take = remaining
		}

		cost += take * level.Price
		filled += take
		remaining -= take
	}

	if filled <= 0 {
		return 0, 0, fmt.Errorf("bookstore: insufficient depth for %s", symbol)
	}

	return cost / filled, filled, nil
}

func (store *BookStore) bookSlot(symbol string) *atomic.Pointer[BookUpdate] {
	raw, loaded := store.books.Load(symbol)

	if loaded {
		return raw.(*atomic.Pointer[BookUpdate])
	}

	created := &atomic.Pointer[BookUpdate]{}
	actual, _ := store.books.LoadOrStore(symbol, created)

	return actual.(*atomic.Pointer[BookUpdate])
}

func (store *BookStore) verify(symbol string, book *BookUpdate) error {
	expected := book.Checksum

	if expected == 0 {
		return nil
	}

	computed := checksumBook(book, store.checksumLevels)

	if computed != uint32(expected) {
		return fmt.Errorf(
			"bookstore: checksum mismatch for %s expected %d computed %d",
			symbol,
			expected,
			computed,
		)
	}

	return nil
}

func cloneBookUpdate(update *BookUpdate) *BookUpdate {
	if update == nil {
		return nil
	}

	clone := *update
	clone.Bids = append([]BookLevel(nil), update.Bids...)
	clone.Asks = append([]BookLevel(nil), update.Asks...)

	return &clone
}

func mergeBookUpdate(current *BookUpdate, delta *BookUpdate) *BookUpdate {
	merged := cloneBookUpdate(current)

	if merged == nil {
		return cloneBookUpdate(delta)
	}

	merged.Timestamp = delta.Timestamp
	merged.Checksum = delta.Checksum
	merged.Type = delta.Type
	merged.Bids = mergeLevels(merged.Bids, delta.Bids, true)
	merged.Asks = mergeLevels(merged.Asks, delta.Asks, false)

	return merged
}

func mergeLevels(current []BookLevel, delta []BookLevel, bids bool) []BookLevel {
	levels := make(map[float64]BookLevel)

	for _, level := range current {
		levels[level.Price] = level
	}

	for _, level := range delta {
		if level.Qty <= 0 {
			delete(levels, level.Price)
			continue
		}

		levels[level.Price] = level
	}

	merged := make([]BookLevel, 0, len(levels))

	for _, level := range levels {
		merged = append(merged, level)
	}

	sort.Slice(merged, func(leftIndex int, rightIndex int) bool {
		if bids {
			return merged[leftIndex].Price > merged[rightIndex].Price
		}

		return merged[leftIndex].Price < merged[rightIndex].Price
	})

	return merged
}
