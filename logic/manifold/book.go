package manifold

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
)

/*
BookSource exposes SDK-managed L3 order books for manifold ingestion through a
read lease. PeekBook must hold exclusion against websocket book writers for the
duration of fn — ranging Side.Levels without that lease fatals under concurrent
map write.
*/
type BookSource interface {
	PeekBook(symbol string, fn func(*book.Book)) bool
}

/*
physicalOrder is one resting limit order extracted from the authoritative SDK book.
*/
type physicalOrder struct {
	orderID       string
	side          book.BookDirection
	price         float64
	quantity      float64
	priceMoney    *decimal.Decimal
	quantityMoney *decimal.Decimal
	timestamp     time.Time
}
