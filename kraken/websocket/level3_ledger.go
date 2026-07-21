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
level3Ledger retains Kraken's exact fixed-point order text beside the SDK book.
The SDK decimal remains authoritative for book arithmetic, while checksum input
uses the original text so validation needs neither float conversion nor big.Rat
string reconstruction for every resting order on every update.
*/
type level3Ledger struct {
	orders map[string]map[string]level3Order
}

/*
level3Order is the exact checksum representation of one resting venue order.
*/
type level3Order struct {
	price    string
	quantity string
}

/*
newLevel3Ledger constructs an empty exact-text ledger for one L3 transport.
*/
func newLevel3Ledger() *level3Ledger {
	return &level3Ledger{orders: make(map[string]map[string]level3Order)}
}

/*
Apply decodes one atomic Kraken frame, updates each SDK book with exact decimals,
validates its CRC from retained wire text, then enforces subscribed depth.
*/
func (ledger *level3Ledger) Apply(
	manager *spot.BookManager,
	raw []byte,
) error {
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
applyBook applies every side before validating because Kraken checksums describe
the completed atomic frame, never an intermediate order mutation.
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
		ledger.orders[data.Symbol] = make(map[string]level3Order)
	}

	if managed == nil {
		return fmt.Errorf("level3 book %q is not registered", data.Symbol)
	}

	if ledger.orders[data.Symbol] == nil {
		ledger.orders[data.Symbol] = make(map[string]level3Order)
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
pruneDepth removes exact checksum rows for price levels leaving the subscribed
book before the SDK discards those levels. Kraken no longer sends deletes for
orders outside the requested depth, so retaining them would make the ledger
grow independently of the authoritative live book.
*/
func (ledger *level3Ledger) pruneDepth(
	managed *book.Book,
	symbol string,
) {
	if managed == nil || managed.MaxDepth <= 0 || ledger.orders[symbol] == nil {
		return
	}

	ledger.pruneSide(managed.BestBid(), false, managed.MaxDepth, symbol)
	ledger.pruneSide(managed.BestAsk(), true, managed.MaxDepth, symbol)
}

/*
pruneSide walks from touch to tail and drops every exact order beyond maxDepth.
*/
func (ledger *level3Ledger) pruneSide(
	level *book.Level,
	higher bool,
	maxDepth int,
	symbol string,
) {
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
applySide updates SDK decimal state and its matching exact checksum text from the
same order record, preserving one atomic source for both representations.
*/
func (ledger *level3Ledger) applySide(
	managed *book.Book,
	symbol string,
	direction book.BookDirection,
	orders []kraken.Level3Order,
) error {
	for _, order := range orders {
		if order.LimitPrice == nil {
			return fmt.Errorf("level3 price missing for order %q", order.OrderID)
		}

		quantity := decimal.NewFromInt64(0)

		if order.Event != "delete" {
			if order.OrderQty == nil {
				return fmt.Errorf("level3 quantity missing for order %q", order.OrderID)
			}

			quantity = order.OrderQty
		}

		managed.Update(&book.UpdateOptions{
			Direction: direction,
			ID:        order.OrderID,
			Price:     order.LimitPrice,
			Quantity:  quantity,
			Timestamp: order.Timestamp,
		})

		if order.Event == "delete" {
			delete(ledger.orders[symbol], order.OrderID)
			continue
		}

		ledger.orders[symbol][order.OrderID] = level3Order{
			price:    order.ChecksumLimitPrice(),
			quantity: order.ChecksumOrderQty(),
		}
	}

	return nil
}

/*
checksum streams the best ten ask and bid levels in Kraken queue priority using
the retained exact strings, avoiding aggregate checksum buffers and Rat math.
*/
func (ledger *level3Ledger) checksum(
	managed *book.Book,
	symbol string,
) (uint32, error) {
	checksum := uint32(0)
	var err error

	checksum, err = ledger.writeSide(
		checksum,
		managed.BestAsk(),
		true,
		symbol,
	)

	if err != nil {
		return 0, err
	}

	checksum, err = ledger.writeSide(
		checksum,
		managed.BestBid(),
		false,
		symbol,
	)

	if err != nil {
		return 0, err
	}

	return checksum, nil
}

/*
writeSide visits at most ten price levels and writes every queued order exactly
in the priority order represented by the SDK book.
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
writeChecksumDecimal writes Kraken's decimal-point-free, leading-zero-trimmed
checksum representation directly into the digest without changing its value.
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
