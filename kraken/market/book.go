package market

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"sort"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/theapemachine/errnie"
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

func NewBookParams(symbols []string, depth int) json.RawMessage {
	params := &BookParams{
		Channel:  "book",
		Symbol:   symbols,
		Depth:    depth,
		Snapshot: true,
	}

	raw, err := sonic.Marshal(params)

	if errnie.Error(err) != nil {
		return nil
	}

	return json.RawMessage(raw)
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
	identity  InstrumentIdentity
	bidSide   *bookSide
	askSide   *bookSide
}

/*
SetInstrumentIdentity records the manifold axis mapping for this book feed.
*/
func (book *Book) SetInstrumentIdentity(identity InstrumentIdentity) {
	book.identity = identity
}

/*
InstrumentIdentity returns the manifold axis mapping for this book feed.
*/
func (book *Book) InstrumentIdentity() InstrumentIdentity {
	if book.identity.Symbol == "" && book.Symbol != "" {
		identity, err := SpotIdentityFromPair(book.Symbol)

		if err == nil {
			book.identity = identity
		}
	}

	return book.identity
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

func (book *Book) ensureSides() {
	if book.bidSide == nil {
		book.bidSide = newBidSide()
	}

	if book.askSide == nil {
		book.askSide = newAskSide()
	}
}

func (book *Book) refreshSlices(depth int) {
	book.Bids = book.bidSide.levels(depth)
	book.Asks = book.askSide.levels(depth)
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

	book.ensureSides()

	if update.IsSnapshot() {
		book.Symbol = update.Symbol
		book.bidSide.reset(update.Bids)
		book.askSide.reset(update.Asks)
		book.refreshSlices(depth)
		book.Checksum = update.Checksum
		book.Timestamp = update.Timestamp

		return
	}

	if update.Symbol != "" {
		book.Symbol = update.Symbol
	}

	for _, change := range update.Bids {
		book.bidSide.apply(change)
	}

	for _, change := range update.Asks {
		book.askSide.apply(change)
	}

	book.refreshSlices(depth)
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

	bid := book.Bids[0].Price
	ask := book.Asks[0].Price
	mid = bid + ask
	spread = ask - bid

	for _, level := range book.Bids {
		depth += level.Qty
	}

	for _, level := range book.Asks {
		depth += level.Qty
	}

	return mid, spread, depth, true
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
	if depth <= 0 || len(book.Bids) <= depth {
		return
	}

	book.Bids = book.Bids[:depth]

	if len(book.Asks) > depth {
		book.Asks = book.Asks[:depth]
	}
}

/*
ComputedChecksum returns the CRC32 over the top ten levels per side.
*/
func (book *Book) ComputedChecksum() int64 {
	return int64(book.checksum())
}

func (book *Book) checksum() uint32 {
	payload := book.checksumPayload()

	return crc32.ChecksumIEEE([]byte(payload))
}

func (book *Book) checksumPayload() string {
	var builder strings.Builder

	book.appendChecksumSide(&builder, book.Asks, bookChecksumLevels)
	book.appendChecksumSide(&builder, book.Bids, bookChecksumLevels)

	return builder.String()
}

func (book *Book) appendChecksumSide(builder *strings.Builder, levels []BookLevel, depth int) {
	limit := depth

	if len(levels) < limit {
		limit = len(levels)
	}

	for index := 0; index < limit; index++ {
		level := levels[index]
		priceRaw := level.PriceRaw
		qtyRaw := level.QtyRaw

		if priceRaw == "" {
			priceRaw = strconvFormatChecksum(level.Price)
		}

		if qtyRaw == "" {
			qtyRaw = strconvFormatChecksum(level.Qty)
		}

		builder.WriteString(formatChecksumToken(priceRaw))
		builder.WriteString(formatChecksumToken(qtyRaw))
	}
}

func formatChecksumToken(raw string) string {
	withoutDot := strings.ReplaceAll(raw, ".", "")
	trimmed := strings.TrimLeft(withoutDot, "0")

	if trimmed == "" {
		return "0"
	}

	return trimmed
}

func strconvFormatChecksum(value float64) string {
	return fmt.Sprintf("%g", value)
}

func (book *Book) cloneLevels(levels []BookLevel) []BookLevel {
	if len(levels) == 0 {
		return nil
	}

	out := make([]BookLevel, len(levels))
	copy(out, levels)

	return out
}
