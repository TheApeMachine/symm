package strategy

import (
	"math/big"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

/*
	Reprice validates the frozen claim against current spot depth and current venue

rules immediately before placement. Exact equality/budget bounds replace an
arbitrary slippage tolerance; a changed claim must be proposed anew.
*/
func (candidate *EntryCandidate) Reprice(books LearningBook, pair kraken.InstrumentPair, fee *kraken.TradeVolumeFee, at time.Time) (*big.Rat, string) {
	if !candidate.Current(at) {
		return nil, "stale"
	}

	if pair.Symbol != candidate.Record.Symbol || fee == nil || fee.Fee == nil {
		return nil, "no longer executable"
	}
	wallet := virtualWallet{}
	if err := wallet.initialize(decimal.NewFromInt64(1), pair, fee.Fee); err != nil {
		return nil, err.Error()
	}
	wallet.cash.Set(candidate.cost)

	if wallet.pricing.Minimum.RatString() != candidate.Record.QtyMinimum || wallet.pricing.Lot.RatString() != candidate.Record.QtyIncrement || wallet.pricing.CostMinimum.RatString() != candidate.Record.CostMinimum || wallet.pricing.Rate.RatString() != candidate.Record.FeeRate {
		return nil, "no longer executable"
	}
	var cost *big.Rat
	books.Book(pair.Symbol, func(book *spotbook.Book) {
		if book == nil || book.Bids == nil || book.Asks == nil || book.Bids.High == nil || book.Asks.Low == nil || book.Bids.High.Price.Cmp(book.Asks.Low.Price) >= 0 {
			return
		}
		quantity, gross := wallet.pricing.Sweep(book, candidate.quantity, &wallet.cash, true, nil, nil)

		if quantity.Cmp(candidate.quantity) != 0 {
			return
		}
		priced := wallet.pricing.Total(new(big.Rat), gross, true)

		if priced.Cmp(candidate.cost) > 0 || book.Bids.High.Price.Rat().Cmp(candidate.bid) < 0 {
			return
		}
		cost = priced
	})

	if cost == nil {
		return nil, "repricing failed"
	}
	return cost, ""
}
