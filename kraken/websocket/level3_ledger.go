package websocket

import (
	"fmt"
	"hash/crc32"

	"github.com/bytedance/sonic"
	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/symm/kraken"
)

/*
level3Ledger retains exact L3 order text keyed by order ID so checksum input and
book mutations stay aligned even when one order moves between price levels.
*/
type level3Ledger struct {
	orders  map[string]map[string]level3Order
	waiting map[string]struct{}
}

type level3Order struct {
	price    string
	quantity string
}

/*
newLevel3Ledger creates a per-symbol order cache plus snapshot waiting state.
*/
func newLevel3Ledger() *level3Ledger {
	return &level3Ledger{
		orders:  make(map[string]map[string]level3Order),
		waiting: make(map[string]struct{}),
	}
}

/*
Apply decodes one raw frame and routes each payload row into book mutations.
*/
func (ledger *level3Ledger) Apply(manager *spot.BookManager, raw []byte) error {
	var frame kraken.Level3

	if err := sonic.Unmarshal(raw, &frame); err != nil {
		return fmt.Errorf("level3 decode: %w", err)
	}

	for index := range frame.Data {
		data := &frame.Data[index]

		if data.Type == "" {
			data.Type = frame.Type
		}

		if err := ledger.applyBook(manager, data); err != nil {
			return err
		}
	}

	return nil
}

/*
applyBook applies one symbol payload by enforcing snapshot ordering and checksum
validation before depth pruning.
*/
func (ledger *level3Ledger) applyBook(
	manager *spot.BookManager,
	data *kraken.Level3Data,
) error {
	managed := manager.GetBook(data.Symbol)

	if data.Type == "snapshot" {
		if managed == nil {
			return fmt.Errorf("level3 book %q is not registered", data.Symbol)
		}

		managed = manager.CreateBook(data.Symbol, managed.MaxDepth)
		managed.EnableMaxDepth = false
		managed.NoBookCrossing = false
		ledger.orders[data.Symbol] = make(map[string]level3Order)
		delete(ledger.waiting, data.Symbol)
	}

	if managed == nil {
		return fmt.Errorf("level3 book %q is not registered", data.Symbol)
	}

	if data.Type != "snapshot" {
		if _, waiting := ledger.waiting[data.Symbol]; waiting || ledger.orders[data.Symbol] == nil {
			ledger.waiting[data.Symbol] = struct{}{}
			return nil
		}
	}

	if err := ledger.applySide(managed, data.Symbol, book.Bid, data.Bids); err != nil {
		return err
	}

	if err := ledger.applySide(managed, data.Symbol, book.Ask, data.Asks); err != nil {
		return err
	}

	checksum, err := ledger.checksum(managed, data.Symbol)

	if err != nil {
		return err
	}

	if checksum != data.Checksum {
		return fmt.Errorf(
			"level3 checksum failed, server %d versus local %d",
			data.Checksum,
			checksum,
		)
	}

	ledger.pruneDepth(managed, data.Symbol)
	managed.EnforceDepth()

	return nil
}

/*
pruneDepth keeps each side of the managed book aligned with the configured
maxDepth and the per-symbol order cache.
*/
func (ledger *level3Ledger) pruneDepth(managed *book.Book, symbol string) {
	if managed == nil || managed.MaxDepth <= 0 || ledger.orders[symbol] == nil {
		return
	}

	ledger.pruneSide(managed.BestBid(), false, managed.MaxDepth, symbol)
	ledger.pruneSide(managed.BestAsk(), true, managed.MaxDepth, symbol)
}

/*
pruneSide discards order-cache entries past the configured depth on one side.
*/
func (ledger *level3Ledger) pruneSide(level *book.Level, higher bool, maxDepth int, symbol string) {
	for depth := 0; level != nil; depth++ {
		next := level.Lower

		if higher {
			next = level.Higher
		}

		if depth >= maxDepth {
			for _, order := range level.Queue() {
				delete(ledger.orders[symbol], order.ID)
			}
		}

		level = next
	}
}

/*
applySide mutates one side of the book with add/modify/delete semantics and
tracks exact per-order text for checksum replay safety.
*/
func (ledger *level3Ledger) applySide(
	managed *book.Book,
	symbol string,
	direction book.BookDirection,
	orders []kraken.Level3Order,
) error {
	for _, order := range orders {
		if order.OrderID == "" {
			return fmt.Errorf("level3 order_id missing")
		}

		event := order.Event

		if event == "" {
			event = "add"
		}

		current, exists := ledger.orders[symbol][order.OrderID]

		switch event {
		case "add":
			if exists {
				return fmt.Errorf("level3 add after add for order %q", order.OrderID)
			}

			if order.LimitPrice == nil {
				return fmt.Errorf("level3 price missing for order %q", order.OrderID)
			}

			if order.OrderQty == nil {
				return fmt.Errorf("level3 quantity missing for order %q", order.OrderID)
			}

			managed.Update(&book.UpdateOptions{
				Direction: direction,
				ID:        order.OrderID,
				Price:     order.LimitPrice,
				Quantity:  order.OrderQty,
				Timestamp: order.Timestamp,
			})

			ledger.orders[symbol][order.OrderID] = level3Order{
				price:    order.ChecksumLimitPrice(),
				quantity: order.ChecksumOrderQty(),
			}
		case "modify":
			if !exists {
				return fmt.Errorf("level3 modify before add for order %q", order.OrderID)
			}

			if order.LimitPrice == nil {
				return fmt.Errorf("level3 price missing for order %q", order.OrderID)
			}

			if order.OrderQty == nil {
				return fmt.Errorf("level3 quantity missing for order %q", order.OrderID)
			}

			previousPrice, err := decimal.NewFromString(current.price)

			if err != nil {
				return fmt.Errorf("level3 prior price invalid for order %q: %w", order.OrderID, err)
			}

			if previousPrice.Cmp(order.LimitPrice) != 0 {
				managed.Update(&book.UpdateOptions{
					Direction: direction,
					ID:        order.OrderID,
					Price:     previousPrice,
					Quantity:  decimal.NewFromInt64(0),
					Timestamp: order.Timestamp,
				})
			}

			managed.Update(&book.UpdateOptions{
				Direction: direction,
				ID:        order.OrderID,
				Price:     order.LimitPrice,
				Quantity:  order.OrderQty,
				Timestamp: order.Timestamp,
			})

			ledger.orders[symbol][order.OrderID] = level3Order{
				price:    order.ChecksumLimitPrice(),
				quantity: order.ChecksumOrderQty(),
			}
		case "delete":
			if !exists {
				return fmt.Errorf("level3 delete before add for order %q", order.OrderID)
			}

			previousPrice, err := decimal.NewFromString(current.price)

			if err != nil {
				return fmt.Errorf("level3 prior price invalid for order %q: %w", order.OrderID, err)
			}

			managed.Update(&book.UpdateOptions{
				Direction: direction,
				ID:        order.OrderID,
				Price:     previousPrice,
				Quantity:  decimal.NewFromInt64(0),
				Timestamp: order.Timestamp,
			})

			delete(ledger.orders[symbol], order.OrderID)
		default:
			return fmt.Errorf("level3 unknown event %q for order %q", event, order.OrderID)
		}
	}

	return nil
}

/*
checksum recomputes the per-symbol book checksum from cached level text.
*/
func (ledger *level3Ledger) checksum(managed *book.Book, symbol string) (uint32, error) {
	checksum := uint32(0)
	var err error

	checksum, err = ledger.writeSide(checksum, managed.BestAsk(), true, symbol)

	if err != nil {
		return 0, err
	}

	checksum, err = ledger.writeSide(checksum, managed.BestBid(), false, symbol)

	if err != nil {
		return 0, err
	}

	return checksum, nil
}

/*
writeSide writes up to ten price levels from one side into the checksum.
*/
func (ledger *level3Ledger) writeSide(
	checksum uint32,
	level *book.Level,
	ask bool,
	symbol string,
) (uint32, error) {
	for range 10 {
		if level == nil {
			return checksum, nil
		}

		for _, order := range level.Queue() {
			exact, ok := ledger.orders[symbol][order.ID]

			if !ok {
				return checksum, fmt.Errorf(
					"level3 checksum text missing for order %q",
					order.ID,
				)
			}

			checksum = writeChecksumDecimal(checksum, exact.price)
			checksum = writeChecksumDecimal(checksum, exact.quantity)
		}

		if ask {
			level = level.Higher
			continue
		}

		level = level.Lower
	}

	return checksum, nil
}

/*
writeChecksumDecimal accumulates the kraken checksum input after trimming dot and
leading zeroes according to the protocol text format.
*/
func writeChecksumDecimal(checksum uint32, value string) uint32 {
	started := false
	var next [1]byte

	for index := range len(value) {
		character := value[index]

		if character == '.' {
			continue
		}

		if !started && character == '0' {
			continue
		}

		started = true
		next[0] = character
		checksum = crc32.Update(checksum, crc32.IEEETable, next[:])
	}

	return checksum
}
