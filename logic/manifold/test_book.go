package manifold

import (
	"iter"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
)

/*
testBookSource serves one in-memory L3 book for GPU integration tests.
*/
type testBookSource struct {
	manager *spot.BookManager
	symbol  string
}

func newTestBookSource(symbol string) *testBookSource {
	manager := spot.NewBookManager()
	symbolBook := manager.CreateBook(symbol, 100)
	at := time.Unix(1, 0)

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

	return &testBookSource{
		manager: manager,
		symbol:  symbol,
	}
}

func (source *testBookSource) Books() iter.Seq[*spot.BookManager] {
	return func(yield func(*spot.BookManager) bool) {
		if source == nil || source.manager == nil {
			return
		}

		yield(source.manager)
	}
}
