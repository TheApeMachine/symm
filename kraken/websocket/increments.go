package websocket

import (
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

/*
Increments caches venue price increments from instrument frames so book
rows leave the websocket already stamped for signal math.
*/
type Increments struct {
	bySymbol sync.Map
}

/*
Remember stores each pair's price increment from an instrument frame.
*/
func (cache *Increments) Remember(frame *kraken.Instrument) {
	if frame == nil {
		return
	}

	for index := range frame.Data.Pairs {
		pair := frame.Data.Pairs[index]
		cache.bySymbol.Store(pair.Symbol, pair.PriceIncrement)
	}
}

/*
Stamp writes PriceIncrement onto every book row. Incomplete books are an
error — callers must not Send them.
*/
func (cache *Increments) Stamp(book *kraken.Book) error {
	if book == nil {
		return errnie.Err(errnie.Validation, "websocket: book required to stamp", nil)
	}

	for index := range book.Data {
		value, ok := cache.bySymbol.Load(book.Data[index].Symbol)

		if !ok {
			return errnie.Err(
				errnie.Validation,
				"websocket: price increment required for "+book.Data[index].Symbol,
				nil,
			)
		}

		increment := value.(decimal.Decimal)
		book.Data[index].PriceIncrement = &increment
	}

	return nil
}
