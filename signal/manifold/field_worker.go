package manifold

import (
	"time"

	"github.com/theapemachine/errnie"
	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/compute"
)

func (field *Field) bindSerial(serial *compute.SerialPool) {
	if field == nil {
		return
	}

	field.serial = serial
}

func (field *Field) enqueue(task func()) {
	if field == nil || task == nil {
		return
	}

	if field.serial == nil {
		task()
		return
	}

	field.serial.Enqueue(task)
}

func (field *Field) enqueueTrade(trade *krakenmarket.TradeUpdate, at time.Time) error {
	if trade == nil {
		return nil
	}

	tradeCopy := *trade
	eventAt := at

	field.enqueue(func() {
		if feedErr := field.FeedTrade(&tradeCopy, eventAt); feedErr != nil {
			errnie.Error(feedErr)
		}
	})

	return nil
}

func (field *Field) enqueueTicker(row krakenmarket.TickerUpdate, at time.Time) error {
	rowCopy := row
	eventAt := at

	field.enqueue(func() {
		if feedErr := field.FeedTicker(rowCopy, eventAt); feedErr != nil {
			errnie.Error(feedErr)
		}
	})

	return nil
}

func (field *Field) enqueueBook(update krakenmarket.BookUpdate, at time.Time) error {
	bookCopy := update
	eventAt := at

	field.enqueue(func() {
		if feedErr := field.FeedBook(bookCopy, eventAt); feedErr != nil {
			errnie.Error(feedErr)
		}
	})

	return nil
}

func (field *Field) enqueueFuturesBook(update krakenmarket.BookUpdate, at time.Time) error {
	bookCopy := update
	eventAt := at

	field.enqueue(func() {
		if feedErr := field.FeedFuturesBook(bookCopy, eventAt); feedErr != nil {
			errnie.Error(feedErr)
		}
	})

	return nil
}
