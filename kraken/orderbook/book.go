package orderbook

import (
	"fmt"
	"hash/crc32"
	"slices"
	"strings"
)

// checksumLevels is the number of levels per side Kraken folds into the book
// checksum. It is a protocol constant from the Kraken WebSocket v2 book channel,
// not a tunable: the exchange computes its CRC32 over exactly the top ten asks then
// the top ten bids.
const checksumLevels = 10

/*
MaintainDepth returns the book depth required to reproduce Kraken's book checksum.
Signal tunables may request fewer levels for display or decay weighting, but the
maintained book must keep at least checksumLevels per side or every checksum verify
fails against the exchange's top-ten CRC.
*/
func MaintainDepth(signalDepth int) int {
	if signalDepth < checksumLevels {
		return checksumLevels
	}

	return signalDepth
}

/*
Level is one price level in a maintained book: the resting quantity at a price plus
the exact text the exchange sent for each. The raw text is kept because Kraken's
book checksum is computed over the values as transmitted, at the symbol's own
precision — reformatting a parsed float64 would drop trailing zeros and fail to
reproduce the CRC.
*/
type Level struct {
	Price    float64
	Qty      float64
	PriceRaw string
	QtyRaw   string
}

/*
Book is a locally maintained L2 order book for one symbol.

Kraken delivers an initial snapshot followed by a stream of deltas; a delta carries
only the levels that changed, with a zero quantity meaning "this price is gone."
Treating a delta as a whole book — the bug this type exists to remove — discards
every level the delta did not mention and corrupts every depth-derived signal. Book
applies snapshots and deltas in order, keeps each side sorted and trimmed to depth,
and verifies the running CRC32 the exchange sends so a divergence between the local
book and the exchange is caught rather than silently traded on.
*/
type Book struct {
	depth int
	bids  []Level // best (highest) price first
	asks  []Level // best (lowest) price first
}

/*
NewBook returns an empty book maintained to depth levels per side. depth <= 0 keeps
every level the feed supplies.
*/
func NewBook(depth int) *Book {
	return &Book{depth: depth}
}

/*
ApplySnapshot replaces both sides with a fresh snapshot, sorting and trimming to
depth. It is the resync point: any prior state is discarded.
*/
func (book *Book) ApplySnapshot(bids, asks []Level) {
	book.bids = sortSide(slices.Clone(bids), descending)
	book.asks = sortSide(slices.Clone(asks), ascending)
	book.trim()
}

/*
ApplyDelta folds one delta into the maintained book: a level with a positive
quantity is inserted or updated at its price, a level with zero quantity is removed.
Both sides are re-sorted and trimmed to depth afterwards.
*/
func (book *Book) ApplyDelta(bids, asks []Level) {
	book.bids = mergeSide(book.bids, bids, descending)
	book.asks = mergeSide(book.asks, asks, ascending)
	book.trim()
}

/*
Verify reports whether the maintained book reproduces the exchange checksum. A false
result means the local book has diverged from the exchange — a missed or misapplied
delta — and the caller must resync from a fresh snapshot rather than keep trading on
a book that no longer matches the venue.
*/
func (book *Book) Verify(checksum uint32) bool {
	return book.Checksum() == checksum
}

/*
Checksum computes the Kraken WebSocket v2 book CRC32: the top ten asks followed by
the top ten bids, each level contributing its price then quantity with the decimal
point and leading zeros stripped from the exchange's raw text, all concatenated and
run through CRC32 (IEEE polynomial).
*/
func (book *Book) Checksum() uint32 {
	var builder strings.Builder

	appendChecksumSide(&builder, book.asks)
	appendChecksumSide(&builder, book.bids)

	return crc32.ChecksumIEEE([]byte(builder.String()))
}

/*
Ready reports whether both sides hold at least one level, i.e. the book has received
a snapshot and is usable.
*/
func (book *Book) Ready() bool {
	return len(book.bids) > 0 && len(book.asks) > 0
}

/*
Bids returns a copy of the maintained bid side, best price first.
*/
func (book *Book) Bids() []Level {
	return slices.Clone(book.bids)
}

/*
Asks returns a copy of the maintained ask side, best price first.
*/
func (book *Book) Asks() []Level {
	return slices.Clone(book.asks)
}

func (book *Book) trim() {
	book.bids = truncate(book.bids, book.depth)
	book.asks = truncate(book.asks, book.depth)
}

const (
	ascending  = false
	descending = true
)

func levelPriceKey(level Level) string {
	if level.PriceRaw != "" {
		return level.PriceRaw
	}

	return fmt.Sprintf("%g", level.Price)
}

func mergeSide(current, updates []Level, order bool) []Level {
	byPrice := make(map[string]Level, len(current)+len(updates))

	for _, level := range current {
		byPrice[levelPriceKey(level)] = level
	}

	for _, update := range updates {
		key := levelPriceKey(update)

		if update.Qty == 0 {
			delete(byPrice, key)

			continue
		}

		byPrice[key] = update
	}

	merged := make([]Level, 0, len(byPrice))

	for _, level := range byPrice {
		merged = append(merged, level)
	}

	return sortSide(merged, order)
}

func sortSide(levels []Level, order bool) []Level {
	slices.SortFunc(levels, func(left, right Level) int {
		if order == descending {
			return compareFloat(right.Price, left.Price)
		}

		return compareFloat(left.Price, right.Price)
	})

	return levels
}

func compareFloat(left, right float64) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func truncate(levels []Level, depth int) []Level {
	if depth > 0 && len(levels) > depth {
		return levels[:depth]
	}

	return levels
}

func appendChecksumSide(builder *strings.Builder, levels []Level) {
	count := min(checksumLevels, len(levels))

	for index := range count {
		builder.WriteString(normalizeChecksumToken(levels[index].PriceRaw))
		builder.WriteString(normalizeChecksumToken(levels[index].QtyRaw))
	}
}

/*
normalizeChecksumToken removes the decimal point and any leading zeros from the
exchange's raw numeric text, the canonical form Kraken's CRC32 is computed over.
Trailing zeros are preserved because they encode the symbol's precision and are part
of the checksummed string.
*/
func normalizeChecksumToken(raw string) string {
	withoutPoint := strings.Replace(raw, ".", "", 1)

	return strings.TrimLeft(withoutPoint, "0")
}
