package broker

import (
	"context"
	"fmt"
	"hash/crc32"
	"math/big"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	"github.com/bytedance/sonic"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
)

type Book struct {
	ctx        context.Context
	cancel     context.CancelFunc
	status     *runtime.Status
	pending    atomic.Pointer[map[string]struct{}]
	seeded     chan struct{}
	manager    *spot.BookManager
	normalizer *spot.Normalizer
	notify     atomic.Pointer[func(string, time.Time)]
	touch      atomic.Pointer[func([]kraken.Level3Touch)]
	mu         sync.RWMutex
	stale      sync.Map
	lastTouch  sync.Map
}

func NewBook(ctx context.Context, normalizer *spot.Normalizer) *Book {
	if normalizer == nil {
		panic("websocket: level3 book normalizer required")
	}

	errnie.Info("websocket: initializing book manager")
	ctx, cancel := context.WithCancel(ctx)

	book := &Book{
		ctx:        ctx,
		cancel:     cancel,
		status:     runtime.NewStatus(),
		seeded:     make(chan struct{}, 1),
		manager:    spot.NewBookManager(),
		normalizer: normalizer,
	}

	book.manager.OnCreateBook.Recurring(func(
		event *callback.Event[*spotbook.Book],
	) {
		managed := event.Data

		if managed == nil {
			return
		}

		// Depth belongs to a complete venue frame. Per-order truncation can
		// remove a level that a later order in the same frame still references.
		managed.EnableMaxDepth = false
		// Kraken's checksum is the authority for Level 3 state. Applying the
		// SDK's per-order crossing heuristic inside one multi-order venue frame
		// can delete a newly added order before the later orders in that same
		// frame resolve the transient cross.
		managed.NoBookCrossing = false

		managed.OnChecksummed.Recurring(func(
			bookEvent *callback.Event[*spotbook.ChecksumResult],
		) {
			if !bookEvent.Data.Match {
				book.status.Transition(runtime.ERROR)

				errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf(
						"checksum mismatch for local: %s, and server: %s",
						bookEvent.Data.LocalChecksum,
						bookEvent.Data.ServerChecksum,
					),
					nil,
				))
			}
		})
	})

	return book
}

func (book *Book) Status() runtime.Stage {
	return book.status.Current()
}

/*
Expect keeps the book owner BUSY until every requested snapshot has
been applied and its observation consumers have been seeded.
*/
func (book *Book) Expect(symbols []string) {
	newPending := make(map[string]struct{}, len(symbols))

	for _, symbol := range symbols {
		newPending[symbol] = struct{}{}
	}

	book.pending.Store(&newPending)
	book.status.Transition(runtime.BUSY)
}

/*
Wait blocks boot on the owner's readiness, never on elapsed market time.
*/
func (book *Book) Wait() error {
	for book.Status() != runtime.READY {
		select {
		case <-book.ctx.Done():
			return errnie.Error(errnie.Err(
				errnie.IO, "book: seed interrupted", book.ctx.Err(),
			))
		case <-book.seeded:
		}
	}

	return nil
}

func (book *Book) Book(symbol string, read func(*spotbook.Book)) {
	// Readers (pricing, JSON layers) must not observe a half-applied frame.
	book.mu.RLock()
	defer book.mu.RUnlock()

	if pendingPtr := book.pending.Load(); pendingPtr != nil {
		if _, pending := (*pendingPtr)[symbol]; pending {
			return
		}
	}

	if _, stale := book.stale.Load(symbol); stale {
		return
	}

	managed := book.manager.GetBook(symbol)

	if managed != nil {
		read(managed)
	}
}

func (book *Book) Create(symbol string, depth int) {
	if depth <= 0 {
		depth = viper.GetInt("market.l3_depth")

		if depth <= 0 {
			depth = 10
		}
	}

	book.manager.CreateBook(symbol, depth)
}

/*
Stale marks the books of symbols whose Level3 socket dropped. Their depth is
frozen, so it is an error: readers do not see them until the reconnect
resubscribe delivers a fresh snapshot.
*/
func (book *Book) Stale(symbols []string) {
	for _, symbol := range symbols {
		book.stale.Store(symbol, struct{}{})
	}

	errnie.Error(errnie.Err(
		errnie.IO,
		fmt.Sprintf("level3 socket down, %d books stale until resubscribe snapshot: %s",
			len(symbols), strings.Join(symbols, ",")),
		nil,
	))
}

func (book *Book) SetNotify(notify func(string, time.Time)) {
	book.notify.Store(&notify)
}

func (book *Book) Update(
	payload *kraken.Level3,
) (err error) {
	if payload == nil {
		return nil
	}

	if payload.Data == nil {
		payload.Data = []kraken.Level3Data{}
	}

	book.mu.Lock()
	accepted, applyErr, touches := book.apply(payload)
	book.mu.Unlock()

	var notify func(string, time.Time)

	if notifyPtr := book.notify.Load(); notifyPtr != nil {
		notify = *notifyPtr
	}

	var touch func([]kraken.Level3Touch)

	if touchPtr := book.touch.Load(); touchPtr != nil {
		touch = *touchPtr
	}

	if touch != nil && len(touches) > 0 {
		touch(touches)
	}

	if applyErr != nil {
		return applyErr
	}

	for _, data := range accepted {
		if payload.Type == "snapshot" {
			for {
				oldPendingPtr := book.pending.Load()

				if oldPendingPtr == nil {
					break
				}

				newPending := make(map[string]struct{}, len(*oldPendingPtr))

				for key, val := range *oldPendingPtr {
					if key != data.Symbol {
						newPending[key] = val
					}
				}

				if book.pending.CompareAndSwap(oldPendingPtr, &newPending) {
					break
				}
			}
		}

		if notify != nil {
			notify(data.Symbol, data.Timestamp)
		}

		if payload.Type == "snapshot" {
			pendingCount := 0

			if pendingPtr := book.pending.Load(); pendingPtr != nil {
				pendingCount = len(*pendingPtr)
			}

			staleEmpty := true

			book.stale.Range(func(_, _ any) bool {
				staleEmpty = false
				return false
			})

			if pendingCount == 0 && staleEmpty {
				book.status.Transition(runtime.READY)

				select {
				case book.seeded <- struct{}{}:
				default:
				}
			}
		}
	}

	return nil
}

/*
apply mutates one complete venue frame while the caller owns the book lock.
Transport publication happens only after this method returns and the lock is
released, so downstream backpressure cannot prevent pricing readers from
observing the accepted frame.

A venue checksum mismatch, or a delete that references a level the local book
does not hold, is a bug in local book maintenance. It is raised as an error
carrying the offending entry and the local top of book; the state is neither
wiped nor hidden, so the failure can be reproduced and fixed.
*/
func (book *Book) apply(
	payload *kraken.Level3,
) (accepted []kraken.Level3Data, err error, touches []kraken.Level3Touch) {
	touches = make([]kraken.Level3Touch, 0)
	accepted = make([]kraken.Level3Data, 0, len(payload.Data))

	var symbolBook *spotbook.Book

	for index, data := range payload.Data {
		err := func() error {
			select {
			case <-book.ctx.Done():
				return errnie.Error(book.ctx.Err())
			default:
			}

			data.Type = payload.Type
			original := data
			symbolBook = book.manager.GetBook(data.Symbol)

			depth := viper.GetInt("market.l3_depth")

			if depth <= 0 {
				depth = 10
			}

			if symbolBook == nil {
				symbolBook = book.manager.CreateBook(data.Symbol, depth)
			}

			if payload.Type == "snapshot" {
				book.stale.Delete(data.Symbol)
				symbolBook = book.manager.CreateBook(
					data.Symbol,
					depth,
				)
			}

			for sideIndex, level3data := range []*[]kraken.Level3Order{
				&data.Bids, &data.Asks,
			} {
				symbolSide := symbolBook.Bids
				direction := spotbook.BookDirection(spotbook.Bid)

				if sideIndex == 1 {
					symbolSide = symbolBook.Asks
					direction = spotbook.BookDirection(spotbook.Ask)
				}

				filtered := make([]kraken.Level3Order, 0, len(*level3data))

				for _, order := range *level3data {
					if order.Event == "delete" && order.LimitPrice == nil {
						for _, level := range symbolSide.Levels {
							if level == nil {
								continue
							}

							for _, queued := range level.Queue() {
								if queued.ID == order.OrderID {
									order.LimitPrice = level.Price
									break
								}
							}

							if order.LimitPrice != nil {
								break
							}
						}
					}

					if order.LimitPrice == nil {
						continue
					}

					if order.OrderID != "" {
						for _, level := range symbolSide.Levels {
							if level == nil || level.Price == nil || level.Price.Cmp(order.LimitPrice) == 0 {
								continue
							}

							for _, queued := range level.Queue() {
								if queued.ID == order.OrderID {
									symbolBook.Update(&spotbook.UpdateOptions{
										Direction: direction,
										ID:        order.OrderID,
										Price:     level.Price,
										Quantity:  decimal.NewFromInt64(0),
										Silent:    true,
									})
									break
								}
							}
						}
					}

					quantity := order.OrderQty

					if order.Event == "delete" || quantity == nil {
						quantity = decimal.NewFromInt64(0)
					}

					// The SDK dereferences an absent level on zero-quantity updates.
					// Absence is a lost-book precondition, not an empty order to insert.
					if quantity.Sign() <= 0 && symbolSide.Levels[order.LimitPrice.String()] == nil {
						book.status.Transition(runtime.ERROR)

						return errnie.Err(
							errnie.Validation,
							fmt.Sprintf("level3 %s order %s references absent level %s for %s (frame %s); %s",
								order.Event, order.OrderID, order.LimitPrice.String(), data.Symbol,
								payload.Type, bookContext(symbolBook, original)),
							nil,
						)
					}

					symbolBook.Update(&spotbook.UpdateOptions{
						Direction: direction,
						ID:        order.OrderID,
						Price:     order.LimitPrice,
						Quantity:  quantity,
						Timestamp: order.Timestamp,
						Silent:    true,
					})

					filtered = append(filtered, order)
				}

				*level3data = filtered
			}

			if data.Bids == nil {
				data.Bids = []kraken.Level3Order{}
			}

			if data.Asks == nil {
				data.Asks = []kraken.Level3Order{}
			}

			symbolBook.EnforceDepth()

			payload.Data[index] = data
			if data.Checksum != 0 && !fastL3Checksum(symbolBook, data.Checksum) {
				checksum := symbolBook.L3Checksum(strconv.FormatUint(
					uint64(data.Checksum),
					10,
				))

				if !checksum.Match {
					// The venue checksum is authority: local book maintenance
					// diverged from the venue. Raise it with what is needed to
					// reproduce it; never paper over it.
					return errnie.Err(
						errnie.Validation,
						fmt.Sprintf(
							"level3 checksum mismatch for %s: local %s, server %s (frame %s); %s",
							data.Symbol,
							checksum.LocalChecksum,
							checksum.ServerChecksum,
							payload.Type,
							bookContext(symbolBook, original),
						),
						nil,
					)
				}
			}

			return nil
		}()

		if err != nil {
			return accepted, err, touches
		}

		accepted = append(accepted, data)
		book.recordTouch(symbolBook, data, &touches)
	}

	return accepted, nil, touches
}

/*
bookContext renders what a book-maintenance failure needs for debugging: the
venue entry as received, and the local top ten levels per side with each
order's id, quantity and the price+quantity digits Kraken's level3 checksum
hashes for it.
*/
func bookContext(symbolBook *spotbook.Book, data kraken.Level3Data) string {
	var out strings.Builder

	update, err := sonic.MarshalString(data)

	if err != nil {
		update = "unmarshalable update: " + err.Error()
	}

	out.WriteString("update=" + update)

	if symbolBook == nil {
		out.WriteString(" book=nil")
		return out.String()
	}

	var digits [128]byte

	writeSide := func(name string, cursor *spotbook.Level, next func(*spotbook.Level) *spotbook.Level) {
		out.WriteString(" " + name + "=[")

		for count := 0; count < 10 && cursor != nil; count++ {
			fmt.Fprintf(&out, "%v{", cursor.Price)

			for _, order := range cursor.Queue() {
				hashed := appendDecimalDigits(digits[:0], order.LimitPrice)
				hashed = appendDecimalDigits(hashed, order.Quantity)
				fmt.Fprintf(&out, " %s:%v#%s", order.ID, order.Quantity, hashed)
			}

			out.WriteString(" }")
			cursor = next(cursor)
		}

		out.WriteString("]")
	}

	writeSide("asks", symbolBook.BestAsk(), func(level *spotbook.Level) *spotbook.Level { return level.Higher })
	writeSide("bids", symbolBook.BestBid(), func(level *spotbook.Level) *spotbook.Level { return level.Lower })

	return out.String()
}

/*
SetTouch connects verified top-of-book reporting to the owning transport.
*/
func (book *Book) SetTouch(touch func([]kraken.Level3Touch)) {
	book.touch.Store(&touch)
}

/*
recordTouch stages this symbol's executable top of book if it moved.

The frame reaching here has already matched the venue's checksum, so the levels
are the venue's own. A crossed or one-sided book is not a touch: there is no
price at which both sides could trade, and reporting one would put a number
into the price series that the market never offered.
*/
func (book *Book) recordTouch(symbolBook *spotbook.Book, data kraken.Level3Data, touches *[]kraken.Level3Touch) {
	if symbolBook == nil {
		return
	}
	bid, ask := symbolBook.BestBid(), symbolBook.BestAsk()

	if bid == nil || ask == nil || bid.Price == nil || ask.Price == nil {
		return
	}
	bidPrice, askPrice := bid.Price.Float64(), ask.Price.Float64()

	if bidPrice <= 0 || askPrice <= 0 || bidPrice >= askPrice {
		return
	}

	if val, seen := book.lastTouch.Load(data.Symbol); seen {
		previous := val.([2]float64)
		if previous[0] == bidPrice && previous[1] == askPrice {
			return
		}
	}
	book.lastTouch.Store(data.Symbol, [2]float64{bidPrice, askPrice})

	*touches = append(*touches, kraken.Level3Touch{
		Symbol: data.Symbol, Timestamp: data.Timestamp,
		Bid: bid.Price, BidQty: bid.Quantity,
		Ask: ask.Price, AskQty: ask.Quantity,
	})
}

func (book *Book) All() *sync.Map {
	out := &sync.Map{}
	book.SnapshotInto(out)

	return out
}

func (book *Book) SnapshotInto(out *sync.Map) {
	if book == nil || out == nil {
		return
	}

	for _, symbol := range book.manager.GetBooks() {
		out.Store(symbol, book.manager.GetBook(symbol))
	}
}

/*
JSON returns the serialized JSON representation of the resident book for symbol.
*/
func (book *Book) JSON(symbol string) []byte {
	if book == nil {
		return nil
	}

	var raw []byte

	book.Book(symbol, func(b *spotbook.Book) {
		var err error

		if raw, err = sonic.Marshal(b); err != nil {
			errnie.Error(errnie.Err(errnie.UnprocessableContent, "[book] "+symbol+" marshal failed", err))
			raw = nil
		}
	})

	return raw
}

type decimalLayout struct {
	integer *big.Int
}

func appendDecimalDigits(buf []byte, d *decimal.Decimal) []byte {
	if d == nil {
		return buf
	}

	raw := (*decimalLayout)(unsafe.Pointer(d)).integer

	if raw == nil || raw.Sign() == 0 {
		return buf
	}

	return raw.Append(buf, 10)
}

func fastL3Checksum(b *spotbook.Book, expected uint32) bool {
	var crc uint32
	var buf [64]byte

	cursor := b.BestAsk()

	for count := 0; count < 10 && cursor != nil; count++ {
		for _, order := range cursor.Queue() {
			digits := appendDecimalDigits(buf[:0], order.LimitPrice)
			crc = crc32.Update(crc, crc32.IEEETable, digits)
			digits = appendDecimalDigits(buf[:0], order.Quantity)
			crc = crc32.Update(crc, crc32.IEEETable, digits)
		}

		cursor = cursor.Higher
	}

	cursor = b.BestBid()

	for count := 0; count < 10 && cursor != nil; count++ {
		for _, order := range cursor.Queue() {
			digits := appendDecimalDigits(buf[:0], order.LimitPrice)
			crc = crc32.Update(crc, crc32.IEEETable, digits)
			digits = appendDecimalDigits(buf[:0], order.Quantity)
			crc = crc32.Update(crc, crc32.IEEETable, digits)
		}

		cursor = cursor.Lower
	}

	return crc == expected
}
