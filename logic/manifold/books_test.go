package manifold

import (
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
testBooks is a BookSource backed by real venue books.

The books are the SDK's own spotbook.Book, updated through its own Update path,
so a test exercises the same structure the venue maintains — level ordering,
per-level queues, quantity accounting — rather than a stand-in that agrees with
the projection by construction.
*/
type testBooks struct {
	mu    sync.RWMutex
	books map[string]*spotbook.Book
}

func newTestBooks() *testBooks {
	return &testBooks{books: map[string]*spotbook.Book{}}
}

func (source *testBooks) Books() *sync.Map {
	source.mu.RLock()
	defer source.mu.RUnlock()

	out := &sync.Map{}

	for symbol, book := range source.books {
		out.Store(symbol, book)
	}

	return out
}

func (source *testBooks) Book(symbol string, read func(*spotbook.Book)) {
	source.mu.RLock()
	defer source.mu.RUnlock()

	if book, found := source.books[symbol]; found {
		read(book)
	}
}

/* rest places one order on the book, creating the symbol's book on first use. */
func (source *testBooks) rest(
	symbol string,
	direction spotbook.BookDirection,
	orderID string,
	price float64,
	quantity float64,
) {
	source.mu.Lock()
	defer source.mu.Unlock()

	book, found := source.books[symbol]

	if !found {
		book = spotbook.New()
		book.Name = symbol
		source.books[symbol] = book
	}

	book.Update(&spotbook.UpdateOptions{
		Direction: direction,
		ID:        orderID,
		Price:     decimal.NewFromFloat64(price),
		Quantity:  decimal.NewFromFloat64(quantity),
		Timestamp: time.Now(),
	})
}

/*
pull removes one order by setting its quantity to zero, which is how the venue
retires an order — the book stops listing it, and nothing announces a departure
beyond its absence.
*/
func (source *testBooks) pull(
	symbol string,
	direction spotbook.BookDirection,
	orderID string,
	price float64,
) {
	source.rest(symbol, direction, orderID, price, 0)
}
