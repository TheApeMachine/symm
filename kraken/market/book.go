package market

import (
	"hash"
	"hash/crc32"
	"sort"
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
		book.sortSides()
		book.truncate(depth)
		book.Checksum = update.Checksum
		book.Timestamp = update.Timestamp

		return
	}

	if update.Symbol != "" {
		book.Symbol = update.Symbol
	}

	book.Bids = book.mergeBookSide(book.Bids, update.Bids, false)
	book.Asks = book.mergeBookSide(book.Asks, update.Asks, true)
	book.truncate(depth)
	book.Checksum = update.Checksum
	book.Timestamp = update.Timestamp
}

/*
TouchQuote returns mid proxy, spread, and total resting depth from the folded
top of book. The second return is false when either side is empty.
*/
func (book *Book) TouchQuote() (mid float64, spread float64, depth float64, ok bool) {
	if len(book.Asks) == 0 || len(book.Bids) == 0 {
		return 0, 0, 0, false
	}

	for _, level := range book.Bids {
		depth += level.Qty
	}

	for _, level := range book.Asks {
		depth += level.Qty
	}

	mid = book.Asks[0].Price + book.Bids[0].Price
	spread = book.Asks[0].Price - book.Bids[0].Price

	return mid, spread, depth, true
}

/*
ComputedChecksum returns the Kraken WebSocket v2 CRC32 for this frame's levels.
*/
func (book Book) ComputedChecksum() int64 {
	hasher := crc32.NewIEEE()

	for index := range min(bookChecksumLevels, len(book.Asks)) {
		writeChecksumField(hasher, book.Asks[index].PriceRaw)
		writeChecksumField(hasher, book.Asks[index].QtyRaw)
	}

	for index := range min(bookChecksumLevels, len(book.Bids)) {
		writeChecksumField(hasher, book.Bids[index].PriceRaw)
		writeChecksumField(hasher, book.Bids[index].QtyRaw)
	}

	return int64(hasher.Sum32())
}

func writeChecksumField(hasher hash.Hash32, raw string) {
	started := false
	scratch := []byte{0}

	for index := range len(raw) {
		character := raw[index]

		if character == '.' {
			continue
		}

		if character == '0' && !started {
			continue
		}

		started = true
		scratch[0] = character
		hasher.Write(scratch)
	}

	if !started {
		hasher.Write([]byte{'0'})
	}
}

func (book *Book) sortSides() {
	sort.Slice(book.Bids, func(left, right int) bool {
		return book.Bids[left].Price > book.Bids[right].Price
	})

	sort.Slice(book.Asks, func(left, right int) bool {
		return book.Asks[left].Price < book.Asks[right].Price
	})
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
	levels := existing

	for _, change := range delta {
		levels = book.applyLevelChange(levels, change, askSide)
	}

	return levels
}

func (book *Book) applyLevelChange(
	levels []BookLevel, change BookLevel, askSide bool,
) []BookLevel {
	index := book.levelIndex(levels, change.Price, askSide)

	if change.Qty <= 0 {
		if index < len(levels) && levels[index].Price == change.Price {
			return append(levels[:index], levels[index+1:]...)
		}

		return levels
	}

	if index < len(levels) && levels[index].Price == change.Price {
		levels[index] = change

		return levels
	}

	levels = append(levels, BookLevel{})
	copy(levels[index+1:], levels[index:])
	levels[index] = change

	return levels
}

func (book *Book) levelIndex(levels []BookLevel, price float64, askSide bool) int {
	low := 0
	high := len(levels)

	for low < high {
		mid := low + (high-low)/2

		if askSide {
			if levels[mid].Price < price {
				low = mid + 1
			} else {
				high = mid
			}

			continue
		}

		if levels[mid].Price > price {
			low = mid + 1
		} else {
			high = mid
		}
	}

	return low
}
