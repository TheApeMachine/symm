package websocket

import (
	"fmt"
	"hash/crc32"
	"strings"

	"github.com/krakenfx/api-go/v2/pkg/book"
)

/*
level3Wire retains one resting order's pre-tokenized Kraken L3 checksum text so
validation never re-enters math/big on the websocket read path.
*/
type level3Wire struct {
	token string
}

/*
level3Apply owns wire-text checksum ledgers for SDK books. It applies L3 frames
without BookManager.UpdateL3 so Decimal.String stays off the hot path.
*/
type level3Apply struct {
	wires map[string]map[string]level3Wire
}

/*
newLevel3Apply constructs an empty per-symbol order wire ledger.
*/
func newLevel3Apply() *level3Apply {
	return &level3Apply{wires: make(map[string]map[string]level3Wire)}
}

/*
clear drops cached wire text when a symbol book is (re)created.
*/
func (apply *level3Apply) clear(symbol string) {
	if apply == nil || symbol == "" {
		return
	}

	delete(apply.wires, strings.ToUpper(symbol))
}

/*
remember stores checksum text for one resting order.
*/
func (apply *level3Apply) remember(symbol string, orderID string, wire level3Wire) {
	if apply == nil || symbol == "" || orderID == "" {
		return
	}

	symbol = strings.ToUpper(symbol)
	orders := apply.wires[symbol]

	if orders == nil {
		orders = make(map[string]level3Wire)
		apply.wires[symbol] = orders
	}

	orders[orderID] = wire
}

/*
forget removes checksum text after a delete event.
*/
func (apply *level3Apply) forget(symbol string, orderID string) {
	if apply == nil || symbol == "" || orderID == "" {
		return
	}

	delete(apply.wires[strings.ToUpper(symbol)], orderID)
}

/*
wire returns retained checksum text for one order when present.
*/
func (apply *level3Apply) wire(symbol string, orderID string) (level3Wire, bool) {
	if apply == nil {
		return level3Wire{}, false
	}

	wire, ok := apply.wires[strings.ToUpper(symbol)][orderID]

	return wire, ok
}

/*
checksum builds the Kraken L3 CRC over the top of book using retained wire text,
falling back to Decimal.String only for orders seeded outside the apply path.
*/
func (apply *level3Apply) checksum(symbolBook *book.Book, symbol string) string {
	return fmt.Sprint(crc32.Checksum(
		[]byte(apply.checksumPayload(symbolBook, symbol)),
		crc32.IEEETable,
	))
}

/*
checksumPayload concatenates ask then bid checksum tokens in Kraken walk order.
*/
func (apply *level3Apply) checksumPayload(
	symbolBook *book.Book,
	symbol string,
) string {
	var payload strings.Builder
	cursor := symbolBook.BestAsk()

	for range 10 {
		if cursor == nil {
			break
		}

		for _, order := range cursor.Queue() {
			payload.WriteString(apply.token(symbol, order))
		}

		cursor = cursor.Higher
	}

	cursor = symbolBook.BestBid()

	for range 10 {
		if cursor == nil {
			break
		}

		for _, order := range cursor.Queue() {
			payload.WriteString(apply.token(symbol, order))
		}

		cursor = cursor.Lower
	}

	return payload.String()
}

/*
token returns one order's checksum concatenation from wire text when known.
*/
func (apply *level3Apply) token(symbol string, order *book.Order) string {
	if order == nil {
		return ""
	}

	if wire, ok := apply.wire(symbol, order.ID); ok {
		return wire.token
	}

	price := ""
	quantity := ""

	if order.LimitPrice != nil {
		price = order.LimitPrice.String()
	}

	if order.Quantity != nil {
		quantity = order.Quantity.String()
	}

	return level3ChecksumToken(price, quantity)
}

/*
level3ChecksumToken matches Kraken's trim rules for L3 checksum parts.
*/
func level3ChecksumToken(price string, quantity string) string {
	return strings.TrimLeft(strings.ReplaceAll(price, ".", ""), "0") +
		strings.TrimLeft(strings.ReplaceAll(quantity, ".", ""), "0")
}

/*
level3WireFromText builds a ledger entry from Kraken fixed-point wire strings.
*/
func level3WireFromText(price string, quantity string) level3Wire {
	return level3Wire{token: level3ChecksumToken(price, quantity)}
}
