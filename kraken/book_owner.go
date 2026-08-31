package kraken

import (
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
Side identifies which half of an order book a level belongs to.
*/
type Side string

const (
	SideBid Side = "bid"
	SideAsk Side = "ask"
)

/*
BookOrder is one resident L3 order inside the canonical book. It retains
Kraken's exact fixed-point LimitPrice and OrderQty so executable-liquidation
arithmetic never leaves Kraken's decimal space. OrderID is the exchange order
identity that survives add/modify/delete across frames.
*/
type BookOrder struct {
	OrderID    string
	LimitPrice *decimal.Decimal
	OrderQty   *decimal.Decimal
}

/*
BookView is the immutable, coherent snapshot of one symbol's committed book that
Fold exposes to a read callback. Readers must not retain it beyond the callback:
the callback runs inside the owner's lock, so the slices are only safe to read
during that synchronous closure.
*/
type BookView struct {
	Symbol string
	Valid  bool
	Bids   []BookOrder // sorted descending by LimitPrice (best bid first)
	Asks   []BookOrder // sorted ascending by LimitPrice (best ask first)
}

/*
SymbolBook is the resident, bounded L3 state for one symbol. Each side lives in
a price-ordered slice (bids descending, asks ascending) with an orderID index so
an update mutates in place without re-sorting, and the ordering invariant is
maintained at all times — a reader never sorts or copies the whole structure.
*/
type SymbolBook struct {
	mu     sync.RWMutex
	bids   []BookOrder
	asks   []BookOrder
	bidIdx map[string]int
	askIdx map[string]int
	valid  bool
	saw    bool
	symbol string
}

func newSymbolBook(symbol string) *SymbolBook {
	return &SymbolBook{
		symbol: symbol,
		bidIdx: make(map[string]int),
		askIdx: make(map[string]int),
	}
}

/*
BookOwner is the single authoritative per-symbol L3 book registry. It owns no
goroutines and no per-symbol actors: every mutation is applied under one
per-symbol lock in the caller's goroutine, in causal arrival order, exactly once
per update frame. Readers cannot mutate resident state — the only way to observe
a committed book is through Fold, which never lets a pointer to internal state
escape the callback.
*/
type BookOwner struct {
	mu    sync.RWMutex
	books map[string]*SymbolBook
}

/*
NewBookOwner constructs an empty canonical L3 book owner.
*/
func NewBookOwner() *BookOwner {
	return &BookOwner{books: make(map[string]*SymbolBook)}
}

func (owner *BookOwner) symbolBook(symbol string) *SymbolBook {
	owner.mu.RLock()
	book := owner.books[symbol]
	owner.mu.RUnlock()

	if book != nil {
		return book
	}

	owner.mu.Lock()
	defer owner.mu.Unlock()

	if existing := owner.books[symbol]; existing != nil {
		return existing
	}

	book = newSymbolBook(symbol)
	owner.books[symbol] = book
	return book
}

/*
Apply folds one Level3Data frame into the canonical book. A snapshot replaces
(initializes) the symbol's sides; an update applies every add/modify directly
and removes every delete, each exactly once and in causal order. A crossed or
one-sided result is represented as invalid and is never repaired with invented
state, so a consumer can always distinguish "no usable book" from a coherent
one.
*/
func (owner *BookOwner) Apply(data Level3Data) {
	if owner == nil || data.Symbol == "" {
		return
	}

	book := owner.symbolBook(data.Symbol)
	book.apply(data)
}

func residentOrder(order Level3Order) BookOrder {
	return BookOrder{
		OrderID:    order.OrderID,
		LimitPrice: decimal.NewFromInt64(0).Add(order.LimitPrice),
		OrderQty:   decimal.NewFromInt64(0).Add(order.OrderQty),
	}
}

func (book *SymbolBook) apply(data Level3Data) {
	book.mu.Lock()
	defer book.mu.Unlock()

	if data.Type == "snapshot" || !book.saw {
		book.bids = book.bids[:0]
		book.asks = book.asks[:0]
		clear(book.bidIdx)
		clear(book.askIdx)

		for _, order := range data.Bids {
			if usableOrder(order) {
				book.upsert(order, SideBid)
			}
		}

		for _, order := range data.Asks {
			if usableOrder(order) {
				book.upsert(order, SideAsk)
			}
		}

		book.saw = true
		book.valid = book.coherentLocked()
		return
	}

	// An update frame is a mutation set: apply each order exactly once, in
	// the order it arrived on the wire, against the resident book.
	for _, order := range data.Bids {
		book.applyResident(order, SideBid)
	}

	for _, order := range data.Asks {
		book.applyResident(order, SideAsk)
	}

	book.valid = book.coherentLocked()
}

func usableOrder(order Level3Order) bool {
	return order.OrderID != "" && order.LimitPrice != nil &&
		order.OrderQty != nil && order.LimitPrice.Sign() > 0 &&
		order.OrderQty.Sign() > 0
}

func (book *SymbolBook) applyResident(order Level3Order, side Side) {
	if order.OrderID == "" {
		return
	}

	if order.Event == "delete" {
		book.remove(order.OrderID, side)
		return
	}

	if !usableOrder(order) {
		return
	}

	book.upsert(order, side)
}

func (book *SymbolBook) upsert(order Level3Order, side Side) {
	resident := residentOrder(order)

	if side == SideBid {
		if index, found := book.bidIdx[order.OrderID]; found {
			book.bids[index] = resident
			book.resortBid()
			return
		}

		book.bids = append(book.bids, resident)
		book.reindexBid()
		return
	}

	if index, found := book.askIdx[order.OrderID]; found {
		book.asks[index] = resident
		book.resortAsk()
		return
	}

	book.asks = append(book.asks, resident)
	book.reindexAsk()
}

func (book *SymbolBook) remove(orderID string, side Side) {
	if side == SideBid {
		index, found := book.bidIdx[orderID]

		if !found {
			return
		}

		book.bids = append(book.bids[:index], book.bids[index+1:]...)
		book.reindexBid()
		return
	}

	index, found := book.askIdx[orderID]

	if !found {
		return
	}

	book.asks = append(book.asks[:index], book.asks[index+1:]...)
	book.reindexAsk()
}

/*
reindexBid rebuilds the orderID index and restores the descending-by-price
ordering after a structural (append/remove) change. Because the resident slice
is already nearly sorted and changes are single-order, insertion sort keeps this
cheap while never dropping the ordering invariant.
*/
func (book *SymbolBook) reindexBid() {
	book.bidIdx = make(map[string]int, len(book.bids))
	insertionSort(book.bids, SideBid)

	for index := range book.bids {
		book.bidIdx[book.bids[index].OrderID] = index
	}
}

func (book *SymbolBook) reindexAsk() {
	book.askIdx = make(map[string]int, len(book.asks))
	insertionSort(book.asks, SideAsk)

	for index := range book.asks {
		book.askIdx[book.asks[index].OrderID] = index
	}
}

/*
resortBid restores ordering after an in-place modify at a known index without
reallocating. A modify moves one order; insertion sort repositions it in O(n)
worst case and O(1) when unchanged.
*/
func (book *SymbolBook) resortBid() {
	insertionSort(book.bids, SideBid)

	for index := range book.bids {
		book.bidIdx[book.bids[index].OrderID] = index
	}
}

func (book *SymbolBook) resortAsk() {
	insertionSort(book.asks, SideAsk)

	for index := range book.asks {
		book.askIdx[book.asks[index].OrderID] = index
	}
}

func insertionSort(orders []BookOrder, side Side) {
	for index := 0; index < len(orders); index++ {
		current := orders[index]
		position := index

		for position > 0 && orderedBefore(orders[position-1], current, side) {
			orders[position] = orders[position-1]
			position--
		}

		orders[position] = current
	}
}

func orderedBefore(left, right BookOrder, side Side) bool {
	if side == SideBid {
		// Bids descend: left is out of order when right's price is higher.
		return left.LimitPrice.Cmp(right.LimitPrice) < 0
	}

	// Asks ascend: left is out of order when right's price is lower.
	return left.LimitPrice.Cmp(right.LimitPrice) > 0
}

/*
coherentLocked reports whether the resident sides form a usable, non-crossed,
two-sided book. It runs under the symbol lock.
*/
func (book *SymbolBook) coherentLocked() bool {
	if len(book.bids) == 0 || len(book.asks) == 0 {
		return false
	}

	bestBid := book.bids[0].LimitPrice
	bestAsk := book.asks[0].LimitPrice

	if bestBid == nil || bestAsk == nil {
		return false
	}

	return bestBid.Cmp(bestAsk) < 0
}

/*
Fold invokes read with a coherent, committed view of one symbol's book while
holding the symbol's read lock. It returns whether a book exists for the symbol.
The view (and its slices) must not be retained past the callback: read runs
synchronously inside the lock, and any copy the callback needs is the
callback's own responsibility.
*/
func (owner *BookOwner) Fold(symbol string, read func(BookView)) bool {
	if owner == nil || symbol == "" {
		return false
	}

	owner.mu.RLock()
	book := owner.books[symbol]
	owner.mu.RUnlock()

	if book == nil {
		return false
	}

	book.mu.RLock()
	defer book.mu.RUnlock()

	read(BookView{
		Symbol: symbol,
		Valid:  book.valid,
		Bids:   book.bids,
		Asks:   book.asks,
	})

	return true
}
