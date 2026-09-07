package strategy

import (
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
)

/* Reprice checks current executable cost against the claim's authorized budget. */
func (candidate *EntryCandidate) Reprice(books LearningBook, price *broker.Price, at time.Time) (*decimal.Decimal, string) {
	if !candidate.Current(at) {
		return nil, "stale"
	}
	var cost *decimal.Decimal
	var err error
	books.Book(candidate.Record.Symbol, func(book *spotbook.Book) {
		if book == nil || book.BestBid() == nil || book.BestAsk() == nil {
			return
		}

		if book.BestBid().Price.Cmp(book.BestAsk().Price) >= 0 ||
			!price.Tradable(candidate.Record.Symbol, candidate.quantity, book.BestAsk().Price) {
			return
		}
		quantity, gross, sweepErr := price.Sweep(book, candidate.quantity, candidate.cost, broker.BUY, nil, nil)
		err = sweepErr

		if err != nil || quantity.Cmp(candidate.quantity) != 0 {
			return
		}
		priced := price.WithFee(candidate.Record.Symbol, gross, broker.BUY)

		if priced == nil || priced.Cmp(candidate.cost) > 0 || book.BestBid().Price.Cmp(candidate.bid) < 0 {
			return
		}
		cost = priced
	})

	if err != nil {
		return nil, err.Error()
	}

	if cost == nil {
		return nil, "repricing failed"
	}
	return cost, ""
}
