package websocket

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	ctx        context.Context
	cancel     context.CancelFunc
	status     types.Status
	statusMu   sync.RWMutex
	mu         sync.RWMutex
	resyncMu   sync.Mutex
	resyncing  bool
	resubscribe func()
	manager    *spot.BookManager
	normalizer *spot.Normalizer
	bus        atomic.Pointer[runtime.Workspace]
	notify     func(string)
	emit       func(kraken.Level3Data)
}

/*
SetBus attaches the workspace shared-object pool. Every managed book the
manager creates is shared there under "book:<symbol>" so signals can read the
authoritative Level3 state without reconstructing books themselves.
*/
func (book *Book) SetBus(bus *runtime.Workspace) {
	if book == nil || bus == nil {
		return
	}

	book.bus.Store(bus)
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
		status:     types.INITIALIZING,
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

		managed.EnableMaxDepth = true

		if bus := book.bus.Load(); bus != nil {
			bus.Share("book", managed, managed.Name)
		}

		// Kraken's checksum is the authority for Level 3 state. Applying the
		// SDK's per-order crossing heuristic inside one multi-order venue frame
		// can delete a newly added order before the later orders in that same
		// frame resolve the transient cross.
		managed.NoBookCrossing = false
		book.statusMu.Lock()
		book.status = types.READY
		book.statusMu.Unlock()

		managed.OnChecksummed.Recurring(func(
			bookEvent *callback.Event[*spotbook.ChecksumResult],
		) {
			if !bookEvent.Data.Match {
				book.resyncMu.Lock()
				silenced := book.resyncing
				book.resyncMu.Unlock()

				if !silenced {
					book.statusMu.Lock()
					book.status = types.ERROR
					book.statusMu.Unlock()

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
			}
		})
	})

	return book
}

func (book *Book) Status() types.Status {
	book.statusMu.RLock()
	defer book.statusMu.RUnlock()

	return book.status
}

func (book *Book) Get(symbol string, read func(*spotbook.Book)) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	managed := book.manager.GetBook(symbol)

	if managed != nil {
		read(managed)
	}
}

func (book *Book) Create(symbol string, depth int) {
	book.mu.Lock()
	defer book.mu.Unlock()

	book.manager.CreateBook(symbol, depth)
}

/*
resync coalesces recovery across a flood of checksum mismatches: only the first
diverged frame requests the whole-universe level3 resubscribe; every later frame
in the same divergence window is dropped until the resubscribe completes and a
fresh snapshot resynchronizes the book.
*/
func (book *Book) resync() {
	if book == nil || book.resubscribe == nil {
		return
	}

	book.resyncMu.Lock()

	if book.resyncing {
		book.resyncMu.Unlock()
		return
	}

	book.resyncing = true
	resubscribe := book.resubscribe
	book.resyncMu.Unlock()

	resubscribe()
}

/*
resyncDone releases the coalescing guard once the whole-universe resubscribe and
fresh snapshot have completed, so a subsequent independent divergence can be
recovered rather than dropped forever.
*/
func (book *Book) resyncDone() {
	book.resyncMu.Lock()
	book.resyncing = false
	book.resyncMu.Unlock()
}

func (book *Book) Update(
	event *callback.Event[*sdk.WebSocketMessage],
	payload *kraken.Level3,
) (err error) {
	if event == nil || event.Data == nil || payload == nil {
		return nil
	}

	if payload.Data == nil {
		payload.Data = []kraken.Level3Data{}
	}

	book.mu.Lock()
	accepted, err := book.apply(payload)
	notify := book.notify
	emit := book.emit
	book.mu.Unlock()

	if err != nil {
		return err
	}

	for _, data := range accepted {
		if notify != nil {
			notify(data.Symbol)
		}

		if emit != nil {
			emit(data)
		}
	}

	return nil
}

/*
apply mutates one complete venue frame while the caller owns the book lock.
Transport publication happens only after this method returns and the lock is
released, so downstream backpressure cannot prevent pricing readers from
observing the accepted frame.
*/
func (book *Book) apply(
	payload *kraken.Level3,
) (accepted []kraken.Level3Data, err error) {
	accepted = make([]kraken.Level3Data, 0, len(payload.Data))

	for index, data := range payload.Data {
		data.Type = payload.Type
		symbolBook := book.manager.GetBook(data.Symbol)

		if symbolBook == nil {
			continue
		}

		pair, err := book.normalizer.PairInfo(data.Symbol)

		if err != nil {
			return nil, errnie.Err(
				errnie.Validation,
				fmt.Sprintf("level3 pair metadata for %s", data.Symbol),
				err,
			)
		}

		if payload.Type == "snapshot" {
			symbolBook = book.manager.CreateBook(
				data.Symbol,
				symbolBook.MaxDepth,
			)
		}

		book.pruneNilLevels(symbolBook)

		for sideIndex, level3data := range []*[]kraken.Level3Order{&data.Bids, &data.Asks} {
			symbolSide := symbolBook.Bids
			direction := spotbook.BookDirection(spotbook.Bid)

			if sideIndex == 1 {
				symbolSide = symbolBook.Asks
				direction = spotbook.BookDirection(spotbook.Ask)
			}

			filtered := make([]kraken.Level3Order, 0, len(*level3data))

			for _, order := range *level3data {
				if order.LimitPrice == nil {
					continue
				}

				limitPrice := order.LimitPrice.SetScale(int64(pair.PairDecimals))
				price := limitPrice.String()

				if level, ok := symbolSide.Levels[price]; ok && level == nil {
					delete(symbolSide.Levels, price)
				}

				if order.Event != "delete" {
					if order.OrderQty == nil {
						continue
					}

					orderQuantity := order.OrderQty.SetScale(int64(pair.LotDecimals))

					if orderQuantity.Sign() == 1 {
						symbolBook.Update(&spotbook.UpdateOptions{
							Direction: direction,
							ID:        order.OrderID,
							Price:     limitPrice,
							Quantity:  orderQuantity,
							Timestamp: order.Timestamp,
							Silent:    true,
						})

						filtered = append(filtered, order)
						continue
					}

					if symbolSide.Levels[price] == nil {
						continue
					}

					symbolBook.Update(&spotbook.UpdateOptions{
						Direction: direction,
						ID:        order.OrderID,
						Price:     limitPrice,
						Quantity:  orderQuantity,
						Timestamp: order.Timestamp,
						Silent:    true,
					})

					filtered = append(filtered, order)
					continue
				}

				level := symbolSide.Levels[price]

				if level == nil {
					continue
				}

				var removed *decimal.Decimal

				for _, queued := range level.Queue() {
					if queued.ID == order.OrderID && queued.Quantity != nil {
						removed = decimal.NewFromInt64(0).Sub(queued.Quantity)
						break
					}
				}

				if removed == nil {
					continue
				}

				order.OrderQty = decimal.NewFromInt64(0).Sub(removed)
				symbolBook.Update(&spotbook.UpdateOptions{
					Direction: direction,
					ID:        order.OrderID,
					Price:     limitPrice,
					Quantity:  removed,
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

		payload.Data[index] = data
		if data.Checksum != 0 {
			checksum := symbolBook.L3Checksum(strconv.FormatUint(
				uint64(data.Checksum),
				10,
			))

			if !checksum.Match {
				// Kraken's checksum is the authority for Level 3 state, so a
				// mismatch means this venue book has silently diverged from the
				// exchange. Recovery is to re-run the existing whole-universe
				// level3 subscribe, not to tear down every trading system that
				// happens to read this one symbol. Leave the frame applied but
				// mark the book needing a resync and request one.
				errnie.Error(errnie.Err(
					errnie.Validation,
					fmt.Sprintf(
						"level3 checksum mismatch for %s: local %s, server %s; scheduling recovery",
						data.Symbol,
						checksum.LocalChecksum,
						checksum.ServerChecksum,
					),
					nil,
				))
				book.resync()
			}
		}

		accepted = append(accepted, data)
	}

	return accepted, nil
}

func (book *Book) pruneNilLevels(symbolBook *spotbook.Book) {
	if symbolBook == nil {
		return
	}

	for _, side := range []*spotbook.Side{symbolBook.Bids, symbolBook.Asks} {
		if side == nil {
			continue
		}

		for price, level := range side.Levels {
			if level == nil {
				delete(side.Levels, price)
			}
		}
	}
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

	book.mu.RLock()
	defer book.mu.RUnlock()

	for _, symbol := range book.manager.GetBooks() {
		out.Store(symbol, book.manager.GetBook(symbol))
	}
}
