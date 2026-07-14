package trader

import (
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
)

func seedBookManager(symbol string, bid, ask float64) *spot.BookManager {
	manager := spot.NewBookManager()
	orderBook := manager.CreateBook(symbol, 10)

	orderBook.Update(&book.UpdateOptions{
		Direction: book.Bid,
		ID:        "bid-1",
		Price:     decimal.NewFromFloat64(bid),
		Quantity:  decimal.NewFromFloat64(1),
	})
	orderBook.Update(&book.UpdateOptions{
		Direction: book.Ask,
		ID:        "ask-1",
		Price:     decimal.NewFromFloat64(ask),
		Quantity:  decimal.NewFromFloat64(1),
	})

	return manager
}
