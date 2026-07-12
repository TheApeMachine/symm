package trader

import (
	"hash/crc32"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/logic/manifold"
)

/*
Level3Book maintains a per-symbol local reconstruction of Kraken's level3
order-level book and validates the exchange CRC32 checksum after every update.
Within each price level, Kraken's insertion or amendment timestamp determines
queue priority. A local sequence only breaks equal-timestamp ties
deterministically. Level3's owner goroutine is its only caller, so symbol books
need no internal synchronization.
*/
type Level3Book struct {
	depth int
	books map[string]*level3SymbolBook
}

/*
level3ChecksumLevels is fixed by the Kraken v2 level3 channel contract:
the checksum always covers the top 10 price levels per side regardless
of the subscribed depth.
*/
const level3ChecksumLevels = 10

type level3Order struct {
	limitPrice         float64
	orderQty           float64
	limitPriceChecksum string
	orderQtyChecksum   string
	timestamp          time.Time
	sequence           uint64
}

type level3Entry struct {
	orderID string
	order   level3Order
}

type level3SymbolBook struct {
	bids     map[string]level3Order
	asks     map[string]level3Order
	sequence uint64
	invalid  manifold.InvalidReason
}

/*
NewLevel3Book instantiates a Level3Book that retains at most depth price
levels per side, per symbol, once truncated after each applied update.
*/
func NewLevel3Book(depth int) *Level3Book {
	return &Level3Book{depth: depth, books: map[string]*level3SymbolBook{}}
}

func (level3Book *Level3Book) book(symbol string) *level3SymbolBook {
	book, ok := level3Book.books[symbol]

	if ok {
		return book
	}

	book = &level3SymbolBook{
		bids: map[string]level3Order{},
		asks: map[string]level3Order{},
	}
	level3Book.books[symbol] = book

	return book
}

/*
Apply folds a snapshot or update row into the symbol's local order-level
book, then validates the exchange checksum against the top 10 price
levels on each side. It returns whether the resulting local book is
trustworthy; a false result means the caller must not read TopOfBook for
this symbol until a fresh snapshot restores it. An update order with an
unrecognized event, or a modify/delete targeting an order the local book
never saw, invalidates the book rather than being silently dropped.
*/
func (level3Book *Level3Book) Apply(
	row kraken.Level3Data, pricePrecision, qtyPrecision int,
) bool {
	book := level3Book.book(row.Symbol)

	if strings.EqualFold(row.Type, "snapshot") {
		book.bids = map[string]level3Order{}
		book.asks = map[string]level3Order{}
		book.sequence = 0
	}

	reason := applyLevel3Side(book, book.bids, row.Bids, row.Type)

	if reason == manifold.Valid {
		reason = applyLevel3Side(book, book.asks, row.Asks, row.Type)
	}

	if reason != manifold.Valid {
		book.invalid = reason
		return false
	}

	truncateLevel3(book.bids, level3Book.depth, false)
	truncateLevel3(book.asks, level3Book.depth, true)

	if !verifyLevel3Checksum(book, pricePrecision, qtyPrecision, row.Checksum) {
		book.invalid = manifold.ChecksumFailed
		return false
	}

	book.invalid = manifold.Valid
	return true
}

/*
Invalid reports whether symbol's local level3 state has a checksum,
continuity, or event fault that requires a fresh snapshot.
*/
func (level3Book *Level3Book) Invalid(symbol string) bool {
	return level3Book.InvalidReason(symbol) != manifold.Valid
}

/*
InvalidReason reports the typed continuity fault for symbol's local book.
*/
func (level3Book *Level3Book) InvalidReason(symbol string) manifold.InvalidReason {
	book, ok := level3Book.books[symbol]

	if !ok {
		return manifold.Valid
	}

	return book.invalid
}

/*
TopOfBook returns the best reconstructed bid and ask price for symbol
from the merged local book. ok is false when the symbol is unknown,
either side is empty, or the book has failed checksum validation, so
callers never trade or measure against a book that cannot be trusted.
*/
func (level3Book *Level3Book) TopOfBook(symbol string) (bid, ask float64, ok bool) {
	book, exists := level3Book.books[symbol]

	if !exists {
		return 0, 0, false
	}

	if book.invalid != manifold.Valid || len(book.bids) == 0 || len(book.asks) == 0 {
		return 0, 0, false
	}

	bids := groupLevel3(book.bids, false)
	asks := groupLevel3(book.asks, true)

	return bids[0][0].order.limitPrice, asks[0][0].order.limitPrice, true
}

/*
Reset discards symbol's local order-level book entirely so the next
message is treated as a fresh start. Call this alongside resubscribing
to the level3 channel for a symbol whose checksum has failed, and when a
symbol is demoted out of the trading tier.
*/
func (level3Book *Level3Book) Reset(symbol string) {
	delete(level3Book.books, symbol)
}

func applyLevel3Side(
	book *level3SymbolBook, side map[string]level3Order, orders []kraken.Level3Order, frameType string,
) manifold.InvalidReason {
	if strings.EqualFold(frameType, "snapshot") {
		for _, order := range orders {
			if _, duplicate := side[order.OrderID]; duplicate {
				return manifold.DuplicateOrder
			}

			side[order.OrderID] = level3Order{
				limitPrice:         order.LimitPrice,
				orderQty:           order.OrderQty,
				limitPriceChecksum: order.ChecksumLimitPrice(),
				orderQtyChecksum:   order.ChecksumOrderQty(),
				timestamp:          order.Timestamp,
				sequence:           book.sequence,
			}
			book.sequence++
		}

		return manifold.Valid
	}

	for _, order := range orders {
		reason := applyLevel3Order(book, side, order)

		if reason != manifold.Valid {
			return reason
		}
	}

	return manifold.Valid
}

func applyLevel3Order(
	book *level3SymbolBook,
	side map[string]level3Order,
	order kraken.Level3Order,
) manifold.InvalidReason {
	switch order.Event {
	case "add":
		if _, duplicate := side[order.OrderID]; duplicate {
			return manifold.DuplicateOrder
		}

		side[order.OrderID] = level3Order{
			limitPrice:         order.LimitPrice,
			orderQty:           order.OrderQty,
			limitPriceChecksum: order.ChecksumLimitPrice(),
			orderQtyChecksum:   order.ChecksumOrderQty(),
			timestamp:          order.Timestamp,
			sequence:           book.sequence,
		}
		book.sequence++

		return manifold.Valid
	case "modify":
		existing, ok := side[order.OrderID]

		if !ok {
			return manifold.MissingOrder
		}

		existing.limitPrice = order.LimitPrice
		existing.orderQty = order.OrderQty
		existing.limitPriceChecksum = order.ChecksumLimitPrice()
		existing.orderQtyChecksum = order.ChecksumOrderQty()

		if !order.Timestamp.IsZero() {
			existing.timestamp = order.Timestamp
		}

		side[order.OrderID] = existing

		return manifold.Valid
	case "delete":
		if _, ok := side[order.OrderID]; !ok {
			return manifold.MissingOrder
		}

		delete(side, order.OrderID)
		return manifold.Valid
	default:
		return manifold.UnknownEvent
	}
}

func truncateLevel3(side map[string]level3Order, depth int, ascending bool) {
	if depth <= 0 {
		return
	}

	levels := groupLevel3(side, ascending)

	if len(levels) <= depth {
		return
	}

	for _, level := range levels[depth:] {
		for _, entry := range level {
			delete(side, entry.orderID)
		}
	}
}

/*
groupLevel3 partitions side's orders into price levels ordered by price
(ascending for asks, descending for bids), with orders inside each level
ordered by Kraken's insertion or amendment timestamp.
*/
func groupLevel3(side map[string]level3Order, ascending bool) [][]level3Entry {
	byPrice := map[float64][]level3Entry{}

	for orderID, order := range side {
		byPrice[order.limitPrice] = append(byPrice[order.limitPrice], level3Entry{
			orderID: orderID,
			order:   order,
		})
	}

	prices := make([]float64, 0, len(byPrice))

	for price := range byPrice {
		prices = append(prices, price)
	}

	sort.Slice(prices, func(i, j int) bool {
		if ascending {
			return prices[i] < prices[j]
		}

		return prices[i] > prices[j]
	})

	levels := make([][]level3Entry, len(prices))

	for index, price := range prices {
		entries := byPrice[price]

		sort.Slice(entries, func(left, right int) bool {
			leftOrder := entries[left].order
			rightOrder := entries[right].order

			if leftOrder.timestamp.IsZero() || rightOrder.timestamp.IsZero() ||
				leftOrder.timestamp.Equal(rightOrder.timestamp) {
				return leftOrder.sequence < rightOrder.sequence
			}

			return leftOrder.timestamp.Before(rightOrder.timestamp)
		})

		levels[index] = entries
	}

	return levels
}

func verifyLevel3Checksum(
	book *level3SymbolBook, pricePrecision, qtyPrecision int, expected uint32,
) bool {
	asks := groupLevel3(book.asks, true)
	bids := groupLevel3(book.bids, false)

	var builder strings.Builder

	writeLevel3ChecksumSide(&builder, asks, pricePrecision, qtyPrecision)
	writeLevel3ChecksumSide(&builder, bids, pricePrecision, qtyPrecision)

	return crc32.ChecksumIEEE([]byte(builder.String())) == expected
}

func writeLevel3ChecksumSide(
	builder *strings.Builder, levels [][]level3Entry, pricePrecision, qtyPrecision int,
) {
	for index := range level3ChecksumLevels {
		if index >= len(levels) {
			break
		}

		for _, entry := range levels[index] {
			writeLevel3ChecksumOrder(builder, entry.order, pricePrecision, qtyPrecision)
		}
	}
}

/*
writeLevel3ChecksumOrder appends one order's exact wire price and quantity to
the checksum input, stripping the decimal point and leading zeros per Kraken's
algorithm. Directly constructed Level3Order values have no retained wire text,
so those internal values are explicitly rendered with instrument precision.
*/
func writeLevel3ChecksumOrder(builder *strings.Builder, order level3Order, pricePrecision, qtyPrecision int) {
	builder.WriteString(stripChecksumFormat(level3ChecksumDecimal(
		order.limitPriceChecksum,
		order.limitPrice,
		pricePrecision,
	)))
	builder.WriteString(stripChecksumFormat(level3ChecksumDecimal(
		order.orderQtyChecksum,
		order.orderQty,
		qtyPrecision,
	)))
}

func level3ChecksumDecimal(exact string, value float64, precision int) string {
	if exact != "" {
		return exact
	}

	return strconv.FormatFloat(value, 'f', precision, 64)
}
