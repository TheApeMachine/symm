package websocket

import (
	"context"
	"fmt"
	"strconv"
	"sync"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/callback"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	sdk "github.com/krakenfx/api-go/v2/pkg/kraken"
	"github.com/krakenfx/api-go/v2/pkg/spot"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/runtime"
	"golang.org/x/sync/errgroup"
)

type Book struct {
	ctx        context.Context
	cancel     context.CancelFunc
	status     *runtime.Status
	mu         sync.RWMutex
	manager    *spot.BookManager
	normalizer *spot.Normalizer
	notify     func(string, time.Time)
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
		status:     runtime.NewStatus(),
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

		// Depth belongs to a complete venue frame. Per-order truncation can
		// remove a level that a later order in the same frame still references.
		managed.EnableMaxDepth = false
		// Kraken's checksum is the authority for Level 3 state. Applying the
		// SDK's per-order crossing heuristic inside one multi-order venue frame
		// can delete a newly added order before the later orders in that same
		// frame resolve the transient cross.
		managed.NoBookCrossing = false
		book.status.Transition(runtime.READY)

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

func (book *Book) Book(symbol string, read func(*spotbook.Book)) {
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

	if depth <= 0 {
		depth = viper.GetInt("market.l3_depth")

		if depth <= 0 {
			depth = 10
		}
	}

	book.manager.CreateBook(symbol, depth)
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

func (book *Book) SetNotify(notify func(string, time.Time)) {
	book.mu.Lock()
	book.notify = notify
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
	notify := book.notify
	resync := book.resync
	book.mu.Unlock()

	if len(resynced) > 0 && resync != nil {
		group, ctx := errgroup.WithContext(book.ctx)

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
		return applyErr
	}

	for _, data := range accepted {
		if notify != nil {
			notify(data.Symbol, data.Timestamp)
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

		depth := viper.GetInt("market.l3_depth")

		if depth <= 0 {
			depth = 10
		}

		if symbolBook == nil {
			symbolBook = book.manager.CreateBook(data.Symbol, depth)
		}

		_, diverged := book.diverging[data.Symbol]

		if payload.Type == "snapshot" {
			delete(book.diverging, data.Symbol)
			diverged = false
			symbolBook = book.manager.CreateBook(
				data.Symbol,
				depth,
			)
		}

		if diverged {
			continue
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
					book.diverging[data.Symbol] = struct{}{}
					book.status.Transition(runtime.ERROR)

					return nil, append(resynced, data.Symbol), errnie.Error(errnie.Err(
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
				book.manager.CreateBook(data.Symbol, depth)

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
