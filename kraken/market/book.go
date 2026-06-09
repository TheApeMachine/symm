package market

import (
	"encoding/json"
	"fmt"
	"hash/crc32"
	"sort"
	"strconv"
	"strings"

	"github.com/bytedance/sonic"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
)

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
	return book.Type == "snapshot"
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
		depth = viper.GetInt("market.book.checksum.levels")
	}

	book.ensureSides()

	if update.IsSnapshot() {
		book.Symbol = update.Symbol
		book.bidSide.reset(update.Bids)
		book.askSide.reset(update.Asks)
		book.bidSide.pruneBeyond(depth)
		book.askSide.pruneBeyond(depth)
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

	book.bidSide.pruneBeyond(depth)
	book.askSide.pruneBeyond(depth)
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
CloneMaintained copies the folded L2 state for rollback when a checksum fails.
*/
func (book *Book) CloneMaintained(depth int) Book {
	clone := Book{
		Symbol:    book.Symbol,
		Checksum:  book.Checksum,
		Timestamp: book.Timestamp,
		Type:      book.Type,
	}

	clone.ensureSides()

	if book.bidSide != nil {
		book.bidSide.tree.Ascend(func(level BookLevel) bool {
			clone.bidSide.tree.ReplaceOrInsert(level)
			return true
		})
	}

	if book.askSide != nil {
		book.askSide.tree.Ascend(func(level BookLevel) bool {
			clone.askSide.tree.ReplaceOrInsert(level)
			return true
		})
	}

	clone.refreshSlices(depth)
	return clone
}

/*
ComputedChecksum returns the CRC32 over the top ten levels per side.
*/
func (book *Book) ComputedChecksum() int64 {
	return int64(crc32.ChecksumIEEE([]byte(book.checksumPayload())))
}

func (book *Book) checksumPayload() string {
	var builder strings.Builder
	depth := viper.GetInt("market.book.checksum.levels")

	book.appendChecksumSide(&builder, book.Asks, depth)
	book.appendChecksumSide(&builder, book.Bids, depth)

	return builder.String()
}

func (book *Book) appendChecksumSide(builder *strings.Builder, levels []BookLevel, depth int) {
	limit := depth

	if len(levels) < limit {
		limit = len(levels)
	}

	for index := 0; index < limit; index++ {
		level := levels[index]
		priceRaw, qtyRaw := book.checksumTokens(level)

		builder.WriteString(strings.ReplaceAll(priceRaw, ".", ""))
		builder.WriteString(strings.ReplaceAll(qtyRaw, ".", ""))
	}
}

func (book *Book) checksumTokens(level BookLevel) (priceRaw, qtyRaw string) {
	pair, ok := SharedInstrumentCatalog().Pair(book.Symbol)

	if ok {
		return strconv.FormatFloat(level.Price, 'f', pair.PricePrecision, 64),
			strconv.FormatFloat(level.Qty, 'f', pair.QtyPrecision, 64)
	}

	priceRaw = level.PriceRaw
	qtyRaw = level.QtyRaw

	if priceRaw == "" {
		priceRaw = fmt.Sprintf("%g", level.Price)
	}

	if qtyRaw == "" {
		qtyRaw = fmt.Sprintf("%g", level.Qty)
	}

	return priceRaw, qtyRaw
}
