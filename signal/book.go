package signal

import (
	"context"
	"sync"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/datura/structure"
	"github.com/theapemachine/symm/kraken/market"
)

/*
Book stores scoped book updates on a capped list ring.
OnUpdate runs after each accepted book update.
*/
type Book struct {
	ctx      context.Context
	cancel   context.CancelFunc
	Scope    string
	OnUpdate func(*market.BookUpdate)
	symbols  *sync.Map
}

/*
NewBook returns a book feed backed by a per-symbol list ring.
*/
func NewBook(ctx context.Context) *Book {
	ctx, cancel := context.WithCancel(ctx)

	return &Book{
		ctx:     ctx,
		cancel:  cancel,
		symbols: &sync.Map{},
	}
}

/*
Spread returns the latest top-of-book spread in basis points for the symbol.
*/
func (book *Book) Spread(symbol string) float64 {
	value, ok := book.symbols.Load(symbol)

	if !ok {
		return 0
	}

	ring := value.(*structure.ListRing[*market.BookUpdate])

	var latest *market.BookUpdate

	ring.Do(func(update *market.BookUpdate) {
		if update != nil {
			latest = update
		}
	})

	if latest == nil || len(latest.Bids) == 0 || len(latest.Asks) == 0 {
		return 0
	}

	bid := latest.Bids[0].Price
	ask := latest.Asks[0].Price
	mid := (bid + ask) / 2

	if mid <= 0 {
		return 0
	}

	return (ask - bid) / mid * 10000
}

/*
BookWindow holds one symbol's recent book list-ring window.
*/
type BookWindow struct {
	Latest  *market.BookUpdate
	Prices  []float64
	Spreads []float64
}

/*
Window returns the scoped symbol's book window.
*/
func (book *Book) Window(symbol string) (BookWindow, bool) {
	value, ok := book.symbols.Load(symbol)

	if !ok {
		return BookWindow{}, false
	}

	ring := value.(*structure.ListRing[*market.BookUpdate])

	var window BookWindow

	ring.Do(func(update *market.BookUpdate) {
		if update == nil || len(update.Bids) == 0 || len(update.Asks) == 0 {
			return
		}

		window.Latest = update
		spread := update.Asks[0].Price - update.Bids[0].Price

		if spread <= 0 {
			return
		}

		window.Spreads = append(window.Spreads, spread)

		for _, bid := range update.Bids {
			if bid.Qty > 0 {
				window.Prices = append(window.Prices, bid.Price)
			}
		}

		for _, ask := range update.Asks {
			if ask.Qty > 0 {
				window.Prices = append(window.Prices, ask.Price)
			}
		}
	})

	if window.Latest == nil || len(window.Prices) < 2 || len(window.Spreads) == 0 {
		return BookWindow{}, false
	}

	return window, true
}

/*
Scan visits each book update in the scoped symbol window.
*/
func (book *Book) Scan(symbol string, visit func(*market.BookUpdate)) bool {
	value, ok := book.symbols.Load(symbol)

	if !ok {
		return false
	}

	ring := value.(*structure.ListRing[*market.BookUpdate])
	ring.Do(visit)

	return true
}

func (book *Book) Update(update market.BookUpdates) {
	for _, bookUpdate := range update {
		if bookUpdate == nil || bookUpdate.Symbol == "" {
			continue
		}

		ring, _ := book.symbols.LoadOrStore(
			bookUpdate.Symbol, structure.NewListRing[*market.BookUpdate](
				FeedRingCapacity(),
				datura.Acquire(
					"book", datura.Artifact_Type_json,
				).WithRole("book"),
			),
		)

		ring.(*structure.ListRing[*market.BookUpdate]).Push(bookUpdate)

		if book.OnUpdate != nil {
			book.OnUpdate(bookUpdate)
		}
	}
}

func (book *Book) Read(buffer []byte) (int, error) {
	var total int

	book.symbols.Range(func(key, value any) bool {
		ring := value.(*structure.ListRing[*market.BookUpdate])
		read, err := ring.Read(buffer)

		if err != nil {
			return false
		}

		total += read

		return true
	})

	return total, nil
}

func (book *Book) Close() error {
	book.cancel()

	return nil
}
