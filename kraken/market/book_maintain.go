package market

import (
	"hash/crc32"
	"math"
	"sort"
	"strconv"
	"strings"
)

const bookChecksumLevels = 10

/*
CloneMaintained copies the maintained top-of-book state up to depth levels per side.
*/
func (book *BookUpdate) CloneMaintained(depth int) BookUpdate {
	return BookUpdate{
		Symbol: book.Symbol,
		Type:   book.Type,
		Bids:   cloneBookSide(book.Bids, depth),
		Asks:   cloneBookSide(book.Asks, depth),
	}
}

/*
Fold merges a snapshot or incremental update into the maintained book.
*/
func (book *BookUpdate) Fold(update BookUpdate, depth int) {
	if depth <= 0 {
		return
	}

	if update.Type == "snapshot" || (len(book.Bids) == 0 && len(book.Asks) == 0) {
		book.Symbol = update.Symbol
		book.Type = update.Type
		book.Bids = cloneBookSide(update.Bids, depth)
		book.Asks = cloneBookSide(update.Asks, depth)

		return
	}

	book.Bids = foldBookSide(book.Bids, update.Bids, true, depth)
	book.Asks = foldBookSide(book.Asks, update.Asks, false, depth)
}

/*
ComputedChecksum returns the Kraken WebSocket v2 CRC32 over the top ten levels.
*/
func (book *BookUpdate) ComputedChecksum() int64 {
	payload := checksumPayload(book.Asks, false, bookChecksumLevels) +
		checksumPayload(book.Bids, true, bookChecksumLevels)

	return int64(crc32.ChecksumIEEE([]byte(payload)))
}

func cloneBookSide(levels []BookLevel, depth int) []BookLevel {
	if len(levels) == 0 {
		return nil
	}

	limit := depth

	if len(levels) < limit {
		limit = len(levels)
	}

	out := make([]BookLevel, limit)
	copy(out, levels[:limit])

	return out
}

func foldBookSide(
	current, delta []BookLevel,
	highToLow bool,
	depth int,
) []BookLevel {
	byPrice := make(map[float64]float64, len(current)+len(delta))

	for _, level := range current {
		byPrice[level.Price] = level.Qty
	}

	for _, level := range delta {
		if level.Qty == 0 {
			delete(byPrice, level.Price)

			continue
		}

		byPrice[level.Price] = level.Qty
	}

	if len(byPrice) == 0 {
		return nil
	}

	prices := make([]float64, 0, len(byPrice))

	for price := range byPrice {
		prices = append(prices, price)
	}

	sort.Slice(prices, func(leftIndex, rightIndex int) bool {
		if highToLow {
			return prices[leftIndex] > prices[rightIndex]
		}

		return prices[leftIndex] < prices[rightIndex]
	})

	limit := depth

	if len(prices) < limit {
		limit = len(prices)
	}

	out := make([]BookLevel, limit)

	for index := 0; index < limit; index++ {
		price := prices[index]
		out[index] = BookLevel{Price: price, Qty: byPrice[price]}
	}

	return out
}

func checksumPayload(levels []BookLevel, highToLow bool, depth int) string {
	if len(levels) == 0 {
		return ""
	}

	sorted := append([]BookLevel(nil), levels...)
	sort.Slice(sorted, func(leftIndex, rightIndex int) bool {
		if highToLow {
			return sorted[leftIndex].Price > sorted[rightIndex].Price
		}

		return sorted[leftIndex].Price < sorted[rightIndex].Price
	})

	limit := depth

	if len(sorted) < limit {
		limit = len(sorted)
	}

	var builder strings.Builder

	for index := 0; index < limit; index++ {
		builder.WriteString(checksumToken(sorted[index].Price))
		builder.WriteString(checksumToken(sorted[index].Qty))
	}

	return builder.String()
}

func checksumToken(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return "0"
	}

	token := strings.ReplaceAll(strconv.FormatFloat(value, 'f', -1, 64), ".", "")
	token = strings.TrimLeft(token, "0")

	if token == "" {
		return "0"
	}

	return token
}
