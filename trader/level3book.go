package trader

import (
	"hash/crc32"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/theapemachine/symm/kraken"
)

/*
Level3Book maintains a per-symbol local reconstruction of Kraken's
level3 order-level book from snapshot and per-order add/modify/delete
events, and validates the exchange CRC32 checksum after every update.
Unlike the L2 book, level3 checksums also verify queue priority within
each price level, so every order carries a monotonic arrival sequence
that is preserved across modifies; only a fresh add moves an order to
the back of its price level's queue.
*/
type Level3Book struct {
	depth int
	books *sync.Map
}

/*
level3ChecksumLevels is fixed by the Kraken v2 level3 channel contract:
the checksum always covers the top 10 price levels per side regardless
of the subscribed depth.
*/
const level3ChecksumLevels = 10

type level3Order struct {
	limitPrice float64
	orderQty   float64
	sequence   uint64
}

type level3Entry struct {
	orderID string
	order   level3Order
}

type level3SymbolBook struct {
	bids     map[string]level3Order
	asks     map[string]level3Order
	sequence uint64
	invalid  bool
}

/*
NewLevel3Book instantiates a Level3Book that retains at most depth price
levels per side, per symbol, once truncated after each applied update.
*/
func NewLevel3Book(depth int) *Level3Book {
	return &Level3Book{depth: depth, books: &sync.Map{}}
}

func (level3Book *Level3Book) book(symbol string) *level3SymbolBook {
	value, _ := level3Book.books.LoadOrStore(symbol, &level3SymbolBook{
		bids: map[string]level3Order{},
		asks: map[string]level3Order{},
	})

	return value.(*level3SymbolBook)
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
	}

	valid := applyLevel3Side(book, book.bids, row.Bids, row.Type) &&
		applyLevel3Side(book, book.asks, row.Asks, row.Type)

	if !valid {
		book.invalid = true
		return false
	}

	truncateLevel3(book.bids, level3Book.depth, false)
	truncateLevel3(book.asks, level3Book.depth, true)

	book.invalid = !verifyLevel3Checksum(book, pricePrecision, qtyPrecision, row.Checksum)
	return !book.invalid
}

/*
Invalid reports whether symbol's locally reconstructed level3 book has
failed its most recent checksum validation.
*/
func (level3Book *Level3Book) Invalid(symbol string) bool {
	value, ok := level3Book.books.Load(symbol)

	if !ok {
		return false
	}

	return value.(*level3SymbolBook).invalid
}

/*
TopOfBook returns the best reconstructed bid and ask price for symbol
from the merged local book. ok is false when the symbol is unknown,
either side is empty, or the book has failed checksum validation, so
callers never trade or measure against a book that cannot be trusted.
*/
func (level3Book *Level3Book) TopOfBook(symbol string) (bid, ask float64, ok bool) {
	value, exists := level3Book.books.Load(symbol)

	if !exists {
		return 0, 0, false
	}

	book := value.(*level3SymbolBook)

	if book.invalid || len(book.bids) == 0 || len(book.asks) == 0 {
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
	level3Book.books.Delete(symbol)
}

func applyLevel3Side(
	book *level3SymbolBook, side map[string]level3Order, orders []kraken.Level3Order, frameType string,
) bool {
	if strings.EqualFold(frameType, "snapshot") {
		for _, order := range orders {
			side[order.OrderID] = level3Order{
				limitPrice: order.LimitPrice,
				orderQty:   order.OrderQty,
				sequence:   book.sequence,
			}
			book.sequence++
		}

		return true
	}

	for _, order := range orders {
		if !applyLevel3Order(book, side, order) {
			return false
		}
	}

	return true
}

func applyLevel3Order(book *level3SymbolBook, side map[string]level3Order, order kraken.Level3Order) bool {
	switch order.Event {
	case "add":
		side[order.OrderID] = level3Order{
			limitPrice: order.LimitPrice,
			orderQty:   order.OrderQty,
			sequence:   book.sequence,
		}
		book.sequence++

		return true
	case "modify":
		existing, ok := side[order.OrderID]

		if !ok {
			return false
		}

		existing.limitPrice = order.LimitPrice
		existing.orderQty = order.OrderQty
		side[order.OrderID] = existing

		return true
	case "delete":
		if _, ok := side[order.OrderID]; !ok {
			return false
		}

		delete(side, order.OrderID)
		return true
	default:
		return false
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
(ascending for asks, descending for bids), with the orders inside each
level ordered by arrival sequence to preserve queue priority.
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

		sort.Slice(entries, func(i, j int) bool {
			return entries[i].order.sequence < entries[j].order.sequence
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
writeLevel3ChecksumOrder appends one order's price and quantity to the
checksum input, formatted per Kraken's algorithm: strip the decimal
point, then strip leading zeros. Both fields are rendered at the
instrument's configured price_precision/qty_precision, the exact decimal
width Kraken pads to on the wire, since the local float64 value alone
cannot recover trailing zeros lost during decoding.
*/
func writeLevel3ChecksumOrder(builder *strings.Builder, order level3Order, pricePrecision, qtyPrecision int) {
	builder.WriteString(stripChecksumFormat(strconv.FormatFloat(order.limitPrice, 'f', pricePrecision, 64)))
	builder.WriteString(stripChecksumFormat(strconv.FormatFloat(order.orderQty, 'f', qtyPrecision, 64)))
}
