package broker

import (
	"time"

	"github.com/theapemachine/datura/dmt"
)

/*
Quote is the tree-sourced top-of-book snapshot used for preflight and fill simulation.
*/
type Quote struct {
	Symbol    string
	Bid       float64
	Ask       float64
	Last      float64
	UpdatedAt time.Time
	Book      Book
}

/*
BookLevel is one price/qty level in cached L2 depth.
*/
type BookLevel struct {
	Price float64
	Qty   float64
}

/*
Book holds bid and ask depth copied from the latest tree book row.
*/
type Book struct {
	Bids []BookLevel
	Asks []BookLevel
}

/*
QuoteCache queries ticker/ and book/ tree prefixes per §8 — no relay cache.
*/
type QuoteCache struct {
	tree *dmt.Tree
}

func NewQuoteCache(tree *dmt.Tree) *QuoteCache {
	return &QuoteCache{tree: tree}
}

/*
Mark returns the mid price when bid and ask are present, otherwise last trade.
*/
func (quote Quote) Mark() (float64, bool) {
	if quote.Bid > 0 && quote.Ask > 0 {
		mid := (quote.Bid + quote.Ask) / 2

		if mid > 0 {
			return mid, true
		}
	}

	if quote.Last > 0 {
		return quote.Last, true
	}

	return 0, false
}

/*
QuoteForSymbol returns the merged ticker and book snapshot for one symbol.
*/
func (cache *QuoteCache) QuoteForSymbol(symbol string) (Quote, bool) {
	if cache == nil || cache.tree == nil || symbol == "" {
		return Quote{}, false
	}

	quote := Quote{Symbol: symbol}
	found := false

	for inbound := range cache.tree.Seek([]byte("ticker/" + symbol)) {
		if row, ok := parseTickerArtifact(inbound); ok {
			quote.mergeTicker(row)
			found = true
		}
	}

	for inbound := range cache.tree.Seek([]byte("book/" + symbol)) {
		if row, ok := parseBookArtifact(inbound); ok {
			quote.mergeBook(row)
			found = true
		}
	}

	return quote, found
}

func (quote *Quote) mergeTicker(row tickerRow) {
	if row.bid > 0 {
		quote.Bid = row.bid
	}

	if row.ask > 0 {
		quote.Ask = row.ask
	}

	if row.last > 0 {
		quote.Last = row.last
	}

	if !row.at.IsZero() {
		quote.UpdatedAt = row.at
	}
}

func (quote *Quote) mergeBook(row bookRow) {
	if len(row.bids) > 0 {
		quote.Book.Bids = row.bids
	}

	if len(row.asks) > 0 {
		quote.Book.Asks = row.asks
	}

	if !row.at.IsZero() && quote.UpdatedAt.IsZero() {
		quote.UpdatedAt = row.at
	}
}
