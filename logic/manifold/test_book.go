package manifold

import (
	"iter"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
)

/*
testBookSource serves in-memory L3 books for GPU integration tests.
*/
type testBookSource struct {
	manager *spot.BookManager
}

func newTestBookSource(symbols ...string) *testBookSource {
	manager := spot.NewBookManager()
	at := time.Unix(1, 0)

	for _, symbol := range symbols {
		symbolBook := manager.CreateBook(symbol, 100)
		symbolBook.Update(&book.UpdateOptions{
			Direction: book.Bid,
			ID:        "bid-1",
			Price:     decimal.NewFromFloat64(100),
			Quantity:  decimal.NewFromFloat64(2),
			Timestamp: at,
		})
		symbolBook.Update(&book.UpdateOptions{
			Direction: book.Ask,
			ID:        "ask-1",
			Price:     decimal.NewFromFloat64(101),
			Quantity:  decimal.NewFromFloat64(3),
			Timestamp: at.Add(time.Second),
		})
	}

	return &testBookSource{manager: manager}
}

func (source *testBookSource) Books() iter.Seq[*spot.BookManager] {
	return func(yield func(*spot.BookManager) bool) {
		if source == nil || source.manager == nil {
			return
		}

		yield(source.manager)
	}
}
