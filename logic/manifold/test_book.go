package manifold

import (
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
)

/*
testBookSource serves in-memory L3 books for GPU integration tests. It mirrors
the Live read/write lease so concurrent Update and PeekBook stay race-free.
*/
type testBookSource struct {
	manager *spot.BookManager
	mu      sync.RWMutex
}

func newTestBookSource(symbols ...string) *testBookSource {
	manager := spot.NewBookManager()
	at := time.Unix(1, 0)
	source := &testBookSource{manager: manager}

	for _, symbol := range symbols {
		symbolBook := manager.CreateBook(symbol, 100)
		source.apply(symbolBook, &book.UpdateOptions{
			Direction: book.Bid,
			ID:        "bid-1",
			Price:     decimal.NewFromFloat64(100),
			Quantity:  decimal.NewFromFloat64(2),
			Timestamp: at,
		})
		source.apply(symbolBook, &book.UpdateOptions{
			Direction: book.Ask,
			ID:        "ask-1",
			Price:     decimal.NewFromFloat64(101),
			Quantity:  decimal.NewFromFloat64(3),
			Timestamp: at.Add(time.Second),
		})
	}

	return source
}

/*
PeekBook invokes fn under the test source read lease.
*/
func (source *testBookSource) PeekBook(
	symbol string,
	fn func(*book.Book),
) bool {
	if source == nil || source.manager == nil || fn == nil || symbol == "" {
		return false
	}

	source.mu.RLock()
	defer source.mu.RUnlock()

	symbolBook := source.manager.GetBook(symbol)

	if symbolBook == nil {
		return false
	}

	fn(symbolBook)

	return true
}

/*
apply mutates one book under the write lease used by race tests.
*/
func (source *testBookSource) apply(
	symbolBook *book.Book,
	options *book.UpdateOptions,
) {
	source.mu.Lock()
	defer source.mu.Unlock()

	symbolBook.Update(options)
}
