package trader

import (
	"hash/crc32"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
)

/*
OrderBook maintains a per-symbol local reconstruction of Kraken's L2 book
channel from snapshot and update deltas, and validates the exchange
CRC32 checksum after every update. A book channel "update" message only
carries the levels that changed, so trading code must never read a raw
update row's first bid/ask as the top of book; it must read the merged,
checksum-verified state this type keeps instead.
*/
type OrderBook struct {
	depth int
	books *sync.Map
}

/*
checksumLevels is fixed by the Kraken v2 book channel contract: the
checksum always covers the top 10 levels per side regardless of the
subscribed depth.
*/
const checksumLevels = 10

type orderBookLevel struct {
	price decimal.Decimal
	qty   float64
}

type symbolBook struct {
	bids    map[string]orderBookLevel
	asks    map[string]orderBookLevel
	invalid bool
}

/*
NewOrderBook instantiates an OrderBook that retains at most depth levels
per side, per symbol, once truncated after each applied update.
*/
func NewOrderBook(depth int) *OrderBook {
	return &OrderBook{depth: depth, books: &sync.Map{}}
}

func (orderBook *OrderBook) book(symbol string) *symbolBook {
	value, _ := orderBook.books.LoadOrStore(symbol, &symbolBook{
		bids: map[string]orderBookLevel{},
		asks: map[string]orderBookLevel{},
	})

	return value.(*symbolBook)
}

/*
Apply folds a snapshot or update row into the symbol's local book, then
validates the exchange checksum against the merged top 10 levels on each
side. It returns whether the resulting local book is trustworthy; a
false result means the caller must not read TopOfBook for this symbol
until a fresh snapshot restores it.
*/
func (orderBook *OrderBook) Apply(row kraken.BookData, qtyPrecision int) bool {
	book := orderBook.book(row.Symbol)

	if strings.EqualFold(row.Type, "snapshot") {
		book.bids = map[string]orderBookLevel{}
		book.asks = map[string]orderBookLevel{}
	}

	applySide(book.bids, row.Bids)
	applySide(book.asks, row.Asks)
	truncate(book.bids, orderBook.depth, false)
	truncate(book.asks, orderBook.depth, true)

	book.invalid = !verifyChecksum(book, qtyPrecision, row.Checksum)
	return !book.invalid
}

/*
Invalid reports whether symbol's locally reconstructed book has failed
its most recent checksum validation.
*/
func (orderBook *OrderBook) Invalid(symbol string) bool {
	value, ok := orderBook.books.Load(symbol)

	if !ok {
		return false
	}

	return value.(*symbolBook).invalid
}

/*
TopOfBook returns the best reconstructed bid and ask for symbol from the
merged local book. ok is false when the symbol is unknown, either side is
empty, or the book has failed checksum validation, so callers never trade
or measure against a book that cannot be trusted.
*/
func (orderBook *OrderBook) TopOfBook(symbol string) (bid, ask decimal.Decimal, ok bool) {
	value, exists := orderBook.books.Load(symbol)

	if !exists {
		return decimal.Decimal{}, decimal.Decimal{}, false
	}

	book := value.(*symbolBook)

	if book.invalid || len(book.bids) == 0 || len(book.asks) == 0 {
		return decimal.Decimal{}, decimal.Decimal{}, false
	}

	bids := sortedLevels(book.bids, false)
	asks := sortedLevels(book.asks, true)

	return bids[0].price, asks[0].price, true
}

/*
Reset discards symbol's local book entirely so the next message is
treated as a fresh start. Call this alongside resubscribing to the book
channel for a symbol whose checksum has failed.
*/
func (orderBook *OrderBook) Reset(symbol string) {
	orderBook.books.Delete(symbol)
}

func applySide(side map[string]orderBookLevel, levels []kraken.BookLevel) {
	for _, level := range levels {
		key := level.Price.String()

		if level.Qty <= 0 {
			delete(side, key)
			continue
		}

		side[key] = orderBookLevel{price: level.Price, qty: level.Qty}
	}
}

func truncate(side map[string]orderBookLevel, depth int, ascending bool) {
	if depth <= 0 || len(side) <= depth {
		return
	}

	for _, level := range sortedLevels(side, ascending)[depth:] {
		delete(side, level.price.String())
	}
}

func sortedLevels(side map[string]orderBookLevel, ascending bool) []orderBookLevel {
	levels := make([]orderBookLevel, 0, len(side))

	for _, level := range side {
		levels = append(levels, level)
	}

	sort.Slice(levels, func(i, j int) bool {
		comparison := levels[i].price.Rat().Cmp(levels[j].price.Rat())

		if ascending {
			return comparison < 0
		}

		return comparison > 0
	})

	return levels
}

func verifyChecksum(book *symbolBook, qtyPrecision int, expected uint32) bool {
	asks := sortedLevels(book.asks, true)
	bids := sortedLevels(book.bids, false)

	var builder strings.Builder

	for index := range checksumLevels {
		if index >= len(asks) {
			break
		}

		writeChecksumLevel(&builder, asks[index], qtyPrecision)
	}

	for index := range checksumLevels {
		if index >= len(bids) {
			break
		}

		writeChecksumLevel(&builder, bids[index], qtyPrecision)
	}

	return crc32.ChecksumIEEE([]byte(builder.String())) == expected
}

/*
writeChecksumLevel appends one level's price and quantity to the checksum
input, formatted per Kraken's algorithm: strip the decimal point, then
strip leading zeros. Quantity is rendered at the instrument's configured
qty_precision, the exact decimal width Kraken pads quantities to on the
wire, since the local float64 quantity alone cannot recover trailing
zeros lost during decoding.
*/
func writeChecksumLevel(builder *strings.Builder, level orderBookLevel, qtyPrecision int) {
	builder.WriteString(stripChecksumFormat(level.price.String()))
	builder.WriteString(stripChecksumFormat(strconv.FormatFloat(level.qty, 'f', qtyPrecision, 64)))
}

func stripChecksumFormat(value string) string {
	value = strings.Replace(value, ".", "", 1)
	value = strings.TrimLeft(value, "0")

	if value == "" {
		return "0"
	}

	return value
}
