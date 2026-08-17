package websocket

import (
	"context"
	"fmt"
	"strconv"
	"sync"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

type Book struct {
	ctx        context.Context
	cancel     context.CancelFunc
	status     types.Status
	statusMu   sync.RWMutex
	mu         sync.RWMutex
	manager    *spot.BookManager
	normalizer *spot.Normalizer
	updates    chan<- string
	events     chan<- kraken.Level3Data
	resync     func(string)
	diverging  map[string]struct{}
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
		diverging:  map[string]struct{}{},
	}

	book.manager.OnCreateBook.Recurring(func(
		event *callback.Event[*spotbook.Book],
	) {
		managed := event.Data

		if managed == nil {
			return
		}

		managed.EnableMaxDepth = true
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
SetUpdates connects this cache to the owning transport's coalesced book-update
notification channel.
*/
func (book *Book) SetUpdates(updates chan<- string) {
	book.mu.Lock()
	book.updates = updates
	book.mu.Unlock()
}

/*
SetEvents connects accepted Level 3 frames to the owning transport.
*/
func (book *Book) SetEvents(events chan<- kraken.Level3Data) {
	book.mu.Lock()
	book.events = events
	book.mu.Unlock()
}

/*
SetResync connects checksum-divergence recovery to the owning transport. The
callback owns the venue conversation: unsubscribe the diverged symbol and
resubscribe so the venue delivers a fresh snapshot.
*/
func (book *Book) SetResync(resync func(string)) {
	book.mu.Lock()
	book.resync = resync
	book.mu.Unlock()
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
	accepted, resynced, applyErr := book.apply(payload)
	updates := book.updates
	events := book.events
	resync := book.resync
	book.mu.Unlock()

	if len(resynced) > 0 && resync != nil {
		for _, symbol := range resynced {
			go resync(symbol)
		}
	}

	if applyErr != nil {
		return applyErr
	}

	for _, data := range accepted {
		if updates != nil {
			select {
			case updates <- data.Symbol:
			default:
			}
		}

		if events != nil {
			select {
			case events <- data:
			case <-book.ctx.Done():
				return book.ctx.Err()
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
) (accepted []kraken.Level3Data, resynced []string, err error) {
	accepted = make([]kraken.Level3Data, 0, len(payload.Data))

	for index, data := range payload.Data {
		data.Type = payload.Type
		symbolBook := book.manager.GetBook(data.Symbol)

		if symbolBook == nil {
			continue
		}

		_, diverged := book.diverging[data.Symbol]

		if payload.Type == "snapshot" {
			delete(book.diverging, data.Symbol)
			diverged = false
			symbolBook = book.manager.CreateBook(
				data.Symbol,
				symbolBook.MaxDepth,
			)
		}

		if diverged {
			continue
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

				order.LimitPrice, err = book.normalizer.FormatPrice(
					data.Symbol,
					order.LimitPrice,
				)

				if err != nil {
					return nil, nil, fmt.Errorf(
						"level3 normalize %s price: %w",
						data.Symbol,
						err,
					)
				}

				price := order.LimitPrice.String()

				if level, ok := symbolSide.Levels[price]; ok && level == nil {
					delete(symbolSide.Levels, price)
				}

				if order.Event != "delete" {
					if order.OrderQty == nil {
						continue
					}

					order.OrderQty, err = book.normalizer.FormatSize(
						data.Symbol,
						order.OrderQty,
					)

					if err != nil {
						return nil, nil, fmt.Errorf(
							"level3 normalize %s quantity: %w",
							data.Symbol,
							err,
						)
					}

					if order.OrderQty.Sign() == 1 {
						symbolBook.Update(&spotbook.UpdateOptions{
							Direction: direction,
							ID:        order.OrderID,
							Price:     order.LimitPrice,
							Quantity:  order.OrderQty,
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
						Price:     order.LimitPrice,
						Quantity:  order.OrderQty,
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
					Price:     order.LimitPrice,
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
				// The venue checksum is authority. Local state is known wrong,
				// so it is discarded rather than kept serving corrupt depth,
				// the symbol is marked diverged so later deltas are dropped,
				// and the transport is asked to resubscribe — only a fresh
				// snapshot restores trust.
				book.manager.CreateBook(data.Symbol, symbolBook.MaxDepth)

				if _, marked := book.diverging[data.Symbol]; !marked {
					book.diverging[data.Symbol] = struct{}{}
					resynced = append(resynced, data.Symbol)
				}

				return nil, resynced, errnie.Error(errnie.Err(
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

		accepted = append(accepted, data)
	}

	return accepted, resynced, nil
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
