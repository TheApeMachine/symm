package broker

import (
	"context"
	"fmt"
	"hash/crc32"
	"math/big"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.org/x/sync/errgroup"
)

type Book struct {
	*runtime.System
	pending    atomic.Pointer[map[string]struct{}]
	seeded     chan struct{}
	manager    *spot.BookManager
	normalizer *spot.Normalizer
	notify     atomic.Pointer[func(string, time.Time)]
	resync     atomic.Pointer[func(string)]
	touch      atomic.Pointer[func([]kraken.Level3Touch)]
	mutations  atomic.Pointer[func([]kraken.Level3Data)]
	diverging  sync.Map
	lastTouch  sync.Map
	locks      sync.Map
}

func NewBook(ctx context.Context, normalizer *spot.Normalizer) *Book {
	if normalizer == nil {
		panic("websocket: level3 book normalizer required")
	}

	errnie.Info("websocket: initializing book manager")

	book := &Book{
		seeded:     make(chan struct{}, 1),
		manager:    spot.NewBookManager(),
		normalizer: normalizer,
	}
	book.System = runtime.NewSystem(ctx, "book", book)

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
				book.Transition(runtime.ERROR)

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
	return book.System.Status()
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
	book.Transition(runtime.BUSY)
}

/*
Wait blocks boot on the owner's readiness, never on elapsed market time.
*/
func (book *Book) Wait() error {
	for book.Status() != runtime.READY {
		select {
		case <-book.Context().Done():
			return errnie.Error(errnie.Err(
				errnie.IO, "book: seed interrupted", book.Context().Err(),
			))
		case <-book.seeded:
		}
	}

	return nil
}

/*
Stale marks symbols whose Level3 stream dropped. Their local state stops
tracking the venue the moment the socket goes down, so readers get nothing
(exactly like a checksum-diverged symbol) and deltas are dropped until the
resubscribe delivers a fresh snapshot, which clears the mark. No resync is
requested: the reconnecting transport resubscribes on its own.
*/
func (book *Book) Stale(symbols []string) {
	for _, symbol := range symbols {
		book.diverging.Store(symbol, struct{}{})
	}
}

func (book *Book) symbolLock(symbol string) *sync.RWMutex {
	if val, ok := book.locks.Load(symbol); ok {
		return val.(*sync.RWMutex)
	}

	symbolLock := &sync.RWMutex{}
	actual, _ := book.locks.LoadOrStore(symbol, symbolLock)

	return actual.(*sync.RWMutex)
}

func (book *Book) Book(symbol string, read func(*spotbook.Book)) {
	if pendingPtr := book.pending.Load(); pendingPtr != nil {
		if _, pending := (*pendingPtr)[symbol]; pending {
			return
		}
	}

	if _, diverging := book.diverging.Load(symbol); diverging {
		return
	}

	symbolLock := book.symbolLock(symbol)
	symbolLock.RLock()
	defer symbolLock.RUnlock()

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

	symbolLock := book.symbolLock(symbol)
	symbolLock.Lock()
	defer symbolLock.Unlock()

	book.manager.CreateBook(symbol, depth)
}

/*
SetResync connects checksum-divergence recovery to the owning transport. The
callback owns the venue conversation: unsubscribe the diverged symbol and
resubscribe so the venue delivers a fresh snapshot.
*/
func (book *Book) SetResync(resync func(string)) {
	book.resync.Store(&resync)
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

	accepted, resynced, applyErr, touches := book.apply(payload)

	var notify func(string, time.Time)

	if notifyPtr := book.notify.Load(); notifyPtr != nil {
		notify = *notifyPtr
	}

	var resync func(string)

	if resyncPtr := book.resync.Load(); resyncPtr != nil {
		resync = *resyncPtr
	}

	var touch func([]kraken.Level3Touch)

	if touchPtr := book.touch.Load(); touchPtr != nil {
		touch = *touchPtr
	}

	if touch != nil && len(touches) > 0 {
		touch(touches)
	}

	var mutations func([]kraken.Level3Data)

	if mutationsPtr := book.mutations.Load(); mutationsPtr != nil {
		mutations = *mutationsPtr
	}

	if mutations != nil && len(accepted) > 0 {
		mutations(accepted)
	}

	if len(resynced) > 0 && resync != nil {
		group, ctx := errgroup.WithContext(book.Context())

		for _, symbol := range resynced {
			group.Go(func() error {
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
				}

				resync(symbol)
				return nil
			})
		}
	}

	if applyErr != nil {
		return errnie.Error(applyErr)
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

			divergingEmpty := true

			book.diverging.Range(func(_, _ any) bool {
				divergingEmpty = false
				return false
			})

			if pendingCount == 0 && divergingEmpty {
				book.Transition(runtime.READY)

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

A symbol whose local state has failed a venue checksum stays diverged until a
fresh snapshot replaces it: further deltas would only decorate state that is
already known wrong, so they are dropped. Newly diverged symbols are reported
once, for the owning transport to resubscribe.
*/
func (book *Book) apply(
	payload *kraken.Level3,
) (accepted []kraken.Level3Data, resynced []string, err error, touches []kraken.Level3Touch) {
	touches = make([]kraken.Level3Touch, 0)
	accepted = make([]kraken.Level3Data, 0, len(payload.Data))

	var symbolBook *spotbook.Book

	for index, data := range payload.Data {
		if _, diverged := book.diverging.Load(data.Symbol); diverged && payload.Type != "snapshot" {
			continue
		}

		symbolLock := book.symbolLock(data.Symbol)
		symbolLock.Lock()

		err := func() error {
			defer symbolLock.Unlock()

			select {
			case <-book.Context().Done():
				return errnie.Error(book.Context().Err())
			default:
			}

			data.Type = payload.Type
			symbolBook = book.manager.GetBook(data.Symbol)

			depth := viper.GetInt("market.l3_depth")

			if depth <= 0 {
				depth = 10
			}

			if symbolBook == nil {
				symbolBook = book.manager.CreateBook(data.Symbol, depth)
			}

			if payload.Type == "snapshot" {
				book.diverging.Delete(data.Symbol)
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
						book.manager.CreateBook(data.Symbol, depth)
						book.diverging.Store(data.Symbol, struct{}{})
						book.Transition(runtime.ERROR)
						resynced = append(resynced, data.Symbol)

						return errnie.Error(errnie.Err(
							errnie.Validation,
							fmt.Sprintf("level3 %s order %s references absent level %s for %s; awaiting snapshot",
								order.Event, order.OrderID, order.LimitPrice.String(), data.Symbol),
							nil,
						))
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
					// The venue checksum is authority. Local state is known wrong,
					// so it is discarded rather than kept serving corrupt depth,
					// the symbol is marked diverged so later deltas are dropped,
					// and the book enters ERROR — same halt posture as an absent
					// level. Only a fresh snapshot restores trust.
					book.manager.CreateBook(data.Symbol, depth)
					book.Transition(runtime.ERROR)

					if _, marked := book.diverging.LoadOrStore(data.Symbol, struct{}{}); !marked {
						resynced = append(resynced, data.Symbol)
					}

					return errnie.Error(errnie.Err(
						errnie.Validation,
						fmt.Sprintf(
							"level3 checksum mismatch for %s: local %s, server %s",
							data.Symbol,
							checksum.LocalChecksum,
							checksum.ServerChecksum,
						),
						nil,
					))
				}
			}

			return nil
		}()

		if err != nil {
			return accepted, resynced, err, touches
		}

		accepted = append(accepted, data)
		book.recordTouch(symbolBook, data, &touches)
	}

	return accepted, resynced, nil, touches
}

/*
SetTouch connects verified top-of-book reporting to the owning transport.
*/
func (book *Book) SetTouch(touch func([]kraken.Level3Touch)) {
	book.touch.Store(&touch)
}

/*
SetMutations connects verified checksum-matched level3 order mutations to downstream consumers.
*/
func (book *Book) SetMutations(mutations func([]kraken.Level3Data)) {
	book.mutations.Store(&mutations)
}

/*
ApplyMeasurement updates the book from a persisted Level-3 tape measurement.
It decodes the order mutation and mutates the internal SDK book.
*/
func (book *Book) ApplyMeasurement(measurement *data.Measurement) error {
	if book == nil || measurement == nil {
		return nil
	}

	orderID := measurement.Meta("order_id")

	if orderID == "" {
		return nil
	}

	side := measurement.Meta("side")
	event := measurement.Meta("event")
	orderType := measurement.Meta("type")

	if orderType == "" {
		orderType = "update"
	}

	var priceDec *decimal.Decimal

	if pMetric := data.Pull[*data.MetricEntry](measurement.Read("limit_price")); pMetric.Err == nil {
		if pMetric.Metric.Exact != nil {
			priceDec = pMetric.Metric.Exact
		}

		if priceDec == nil && pMetric.Metric.Raw > 0 {
			priceDec = decimal.NewFromFloat64(pMetric.Metric.Raw)
		}
	}

	var qtyDec *decimal.Decimal

	if qMetric := data.Pull[*data.MetricEntry](measurement.Read("order_qty")); qMetric.Err == nil {
		if qMetric.Metric.Exact != nil {
			qtyDec = qMetric.Metric.Exact
		}

		if qtyDec == nil && qMetric.Metric.Raw >= 0 {
			qtyDec = decimal.NewFromFloat64(qMetric.Metric.Raw)
		}
	}

	var checksum uint32

	if cMetric := data.Pull[*data.MetricEntry](measurement.Read("checksum")); cMetric.Err == nil {
		checksum = uint32(cMetric.Metric.Raw)
	}

	if checksum == 0 {
		if cStr := measurement.Meta("checksum"); cStr != "" {
			if val, err := strconv.ParseUint(cStr, 10, 32); err == nil {
				checksum = uint32(val)
			}
		}
	}

	order := kraken.Level3Order{
		OrderID:    orderID,
		LimitPrice: priceDec,
		OrderQty:   qtyDec,
		Timestamp:  measurement.At,
		Event:      event,
	}

	bids := []kraken.Level3Order{}
	asks := []kraken.Level3Order{}

	if side == "bid" {
		bids = append(bids, order)
	}

	if side == "ask" {
		asks = append(asks, order)
	}

	payload := &kraken.Level3{
		Channel: "level3",
		Type:    orderType,
		Data: []kraken.Level3Data{
			{
				Symbol:    measurement.Label,
				Bids:      bids,
				Asks:      asks,
				Checksum:  checksum,
				Timestamp: measurement.At,
			},
		},
	}

	return book.Update(payload)
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

func fastL3Checksum(managedBook *spotbook.Book, expected uint32) bool {
	var crc uint32
	var buf [64]byte

	cursor := managedBook.BestAsk()

	for count := 0; count < 10 && cursor != nil; count++ {
		for _, order := range cursor.Queue() {
			digits := appendDecimalDigits(buf[:0], order.LimitPrice)
			crc = crc32.Update(crc, crc32.IEEETable, digits)
			digits = appendDecimalDigits(buf[:0], order.Quantity)
			crc = crc32.Update(crc, crc32.IEEETable, digits)
		}

		cursor = cursor.Higher
	}

	cursor = managedBook.BestBid()

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
