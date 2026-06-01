package market

import (
	"hash/crc32"
	"sort"
	"strings"
)

// BookSnapshot is the envelope type tag for a full L2 book frame after subscribe.
const BookSnapshot = "snapshot"

// BookUpdate is the envelope type tag for an incremental L2 book frame.
const BookUpdate = "update"

const bookChecksumLevels = 10

/*
BookParams is the Kraken WebSocket v2 subscribe payload for the book channel.
*/
type BookParams struct {
	Channel  string   `json:"channel"`
	Symbol   []string `json:"symbol"`
	Depth    int      `json:"depth"`
	Snapshot bool     `json:"snapshot"`
}

/*
BookLevel is one price level in an L2 book snapshot or update.
*/
type BookLevel struct {
	Price    float64 `json:"price"`
	Qty      float64 `json:"qty"`
	PriceRaw string  `json:"-"`
	QtyRaw   string  `json:"-"`
}

/*
Book is one L2 order book snapshot or update from the public book WebSocket feed.

Kraken delivers an initial snapshot then incremental updates; each frame carries
bids and asks with aggregated quantity per price, a CRC32 checksum over the top
ten levels per side, and an RFC3339 timestamp. Type records the envelope tag
(snapshot vs update) from the channel message, not the data payload.
*/
type Book struct {
	Symbol    string      `json:"symbol"`
	Bids      []BookLevel `json:"bids"`
	Asks      []BookLevel `json:"asks"`
	Checksum  int64       `json:"checksum"`
	Timestamp string      `json:"timestamp"`
	Type      string      `json:"-"`
}

/*
SetEnvelopeType records the channel envelope tag (snapshot or update).
*/
func (book *Book) SetEnvelopeType(kind string) {
	book.Type = kind
}

/*
IsSnapshot reports whether this frame is a full book snapshot.
*/
func (book *Book) IsSnapshot() bool {
	return book.Type == BookSnapshot
}

/*
Fold merges one Kraken book frame into the receiver per the v2 book channel guide.

Snapshots replace bids and asks; updates merge changed levels and remove entries
with qty zero, then truncate to depth.
*/
func (book *Book) Fold(update Book, depth int) {
	if depth <= 0 {
		depth = bookChecksumLevels
	}

	if update.IsSnapshot() {
		book.Symbol = update.Symbol
		book.Bids = book.cloneLevels(update.Bids)
		book.Asks = book.cloneLevels(update.Asks)
		book.truncate(depth)

		return
	}

	book.Bids = book.mergeBookSide(book.Bids, update.Bids, false)
	book.Asks = book.mergeBookSide(book.Asks, update.Asks, true)
	book.truncate(depth)
}

/*
ComputedChecksum returns the Kraken WebSocket v2 CRC32 for this frame's levels.
*/
func (book Book) ComputedChecksum() int64 {
	var builder strings.Builder

	for index := range min(bookChecksumLevels, len(book.Asks)) {
		builder.WriteString(checksumField(book.Asks[index].PriceRaw))
		builder.WriteString(checksumField(book.Asks[index].QtyRaw))
	}

	for index := range min(bookChecksumLevels, len(book.Bids)) {
		builder.WriteString(checksumField(book.Bids[index].PriceRaw))
		builder.WriteString(checksumField(book.Bids[index].QtyRaw))
	}

	return int64(crc32.ChecksumIEEE([]byte(builder.String())))
}

func checksumField(raw string) string {
	raw = strings.ReplaceAll(raw, ".", "")
	raw = strings.TrimLeft(raw, "0")

	if raw == "" {
		return "0"
	}

	return raw
}

func (book *Book) truncate(depth int) {
	if len(book.Bids) > depth {
		book.Bids = book.Bids[:depth]
	}

	if len(book.Asks) > depth {
		book.Asks = book.Asks[:depth]
	}
}

func (book *Book) cloneLevels(levels []BookLevel) []BookLevel {
	if len(levels) == 0 {
		return nil
	}

	out := make([]BookLevel, len(levels))
	copy(out, levels)

	return out
}

func (book *Book) mergeBookSide(existing, delta []BookLevel, askSide bool) []BookLevel {
	byPrice := make(map[float64]BookLevel, len(existing)+len(delta))

	for _, level := range existing {
		if level.Qty > 0 {
			byPrice[level.Price] = level
		}
	}

	for _, level := range delta {
		if level.Qty <= 0 {
			delete(byPrice, level.Price)

			continue
		}

		byPrice[level.Price] = level
	}

	return book.levelsFromMap(byPrice, askSide)
}

func (book *Book) levelsFromMap(byPrice map[float64]BookLevel, askSide bool) []BookLevel {
	levels := make([]BookLevel, 0, len(byPrice))

	for _, level := range byPrice {
		levels = append(levels, level)
	}

	sort.Slice(levels, func(left, right int) bool {
		if askSide {
			return levels[left].Price < levels[right].Price
		}

		return levels[left].Price > levels[right].Price
	})

	return levels
}
