package manifold

import (
	"time"

	krakenmarket "github.com/theapemachine/symm/kraken/market"
	"github.com/theapemachine/symm/signal/compute"
)

func (field *Field) bindWorker(worker *compute.BatchWorker) {
	if field == nil {
		return
	}

	field.worker = worker
}

func (field *Field) enqueue(task func()) {
	if field == nil || task == nil {
		return
	}

	if field.worker == nil {
		task()
		return
	}

	field.worker.Submit(task)
}

func (field *Field) enqueueTrade(trade *krakenmarket.TradeUpdate, at time.Time) error {
	if trade == nil {
		return nil
	}

	tradeCopy := *trade
	eventAt := at

	field.enqueue(func() {
		_ = field.FeedTrade(&tradeCopy, eventAt)
	})

	return nil
}

func (field *Field) enqueueTicker(row krakenmarket.TickerUpdate, at time.Time) error {
	rowCopy := row
	eventAt := at

	field.enqueue(func() {
		_ = field.FeedTicker(rowCopy, eventAt)
	})

	return nil
}

func (field *Field) enqueueBook(update krakenmarket.BookUpdate, at time.Time) error {
	bookCopy := update
	eventAt := at

	field.enqueue(func() {
		_ = field.FeedBook(bookCopy, eventAt)
	})

	return nil
}

func (field *Field) enqueueFuturesBook(update krakenmarket.BookUpdate, at time.Time) error {
	bookCopy := update
	eventAt := at

	field.enqueue(func() {
		_ = field.FeedFuturesBook(bookCopy, eventAt)
	})

	return nil
}
