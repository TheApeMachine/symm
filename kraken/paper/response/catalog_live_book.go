package response

import (
	"cmp"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/theapemachine/symm/kraken/market"
)

type liveBookSnapshot struct {
	book      depthBook
	updatedAt time.Time
}

/*
ApplyBookUpdate stores one websocket L2 snapshot for paper fill pricing.
*/
func (catalog *PairCatalog) ApplyBookUpdate(update *market.BookUpdate) {
	if catalog == nil || update == nil || update.Symbol == "" {
		return
	}

	if len(update.Bids) == 0 || len(update.Asks) == 0 {
		return
	}

	observedAt := update.Timestamp

	if observedAt.IsZero() {
		observedAt = time.Now()
	}

	catalog.liveBooks.Store(update.Symbol, liveBookSnapshot{
		book:      depthBookFromUpdate(update),
		updatedAt: observedAt,
	})
}

/*
DepthForSymbol returns depth for one ws symbol, preferring a fresh websocket book.
*/
func (catalog *PairCatalog) DepthForSymbol(symbol string, count int) (depthBook, error) {
	if catalog == nil {
		return depthBook{}, fmt.Errorf("paper pair catalog: nil catalog")
	}

	if symbol == "" {
		return depthBook{}, fmt.Errorf("paper pair catalog: symbol missing")
	}

	if count <= 0 {
		return depthBook{}, fmt.Errorf("paper pair catalog: depth count must be positive")
	}

	if book, found := catalog.liveDepthBook(symbol, count); found {
		return book, nil
	}

	restPair, pairErr := catalog.RestPair(symbol)

	if pairErr != nil {
		return depthBook{}, pairErr
	}

	return catalog.DepthBook(restPair, count)
}

func (catalog *PairCatalog) liveDepthBook(symbol string, count int) (depthBook, bool) {
	cacheTTL, ttlErr := catalog.depthCacheTTL()

	if ttlErr != nil || cacheTTL <= 0 {
		return depthBook{}, false
	}

	raw, ok := catalog.liveBooks.Load(symbol)

	if !ok {
		return depthBook{}, false
	}

	entry, entryOK := raw.(liveBookSnapshot)

	if !entryOK {
		catalog.liveBooks.Delete(symbol)
		return depthBook{}, false
	}

	if time.Since(entry.updatedAt) > cacheTTL {
		catalog.liveBooks.Delete(symbol)
		return depthBook{}, false
	}

	return trimDepthBook(entry.book, count), true
}

func depthBookFromUpdate(update *market.BookUpdate) depthBook {
	return depthBook{
		Bids: bookLevelsToDepthRows(update.Bids),
		Asks: bookLevelsToDepthRows(update.Asks),
	}
}

func bookLevelsToDepthRows(levels []market.BookLevel) [][]any {
	rows := make([][]any, 0, len(levels))

	for _, level := range levels {
		if level.Qty <= 0 {
			continue
		}

		rows = append(rows, []any{
			strconv.FormatFloat(level.Price, 'f', -1, 64),
			strconv.FormatFloat(level.Qty, 'f', -1, 64),
		})
	}

	return rows
}

func trimDepthBook(book depthBook, count int) depthBook {
	return depthBook{
		Bids: trimDepthSide(book.Bids, count, true),
		Asks: trimDepthSide(book.Asks, count, false),
	}
}

func trimDepthSide(levels [][]any, count int, bidSide bool) [][]any {
	if count <= 0 || len(levels) == 0 {
		return nil
	}

	rows := slices.Clone(levels)

	slices.SortFunc(rows, func(left, right []any) int {
		leftPrice, leftErr := depthLevelPrice(left)
		rightPrice, rightErr := depthLevelPrice(right)

		if leftErr != nil || rightErr != nil {
			return 0
		}

		if bidSide {
			return cmp.Compare(rightPrice, leftPrice)
		}

		return cmp.Compare(leftPrice, rightPrice)
	})

	if len(rows) > count {
		rows = rows[:count]
	}

	return rows
}

func depthLevelPrice(level []any) (float64, error) {
	if len(level) == 0 {
		return 0, fmt.Errorf("paper pair catalog: depth level missing price")
	}

	priceText, ok := level[0].(string)

	if !ok {
		return 0, fmt.Errorf("paper pair catalog: depth price must be string")
	}

	price, err := strconv.ParseFloat(priceText, 64)

	if err != nil {
		return 0, fmt.Errorf("paper pair catalog: depth price parse: %w", err)
	}

	return price, nil
}
