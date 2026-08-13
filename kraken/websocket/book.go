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
}

func NewBook(ctx context.Context, normalizers ...*spot.Normalizer) *Book {
	errnie.Info("websocket: initializing book manager")
	ctx, cancel := context.WithCancel(ctx)
	var normalizer *spot.Normalizer

	if len(normalizers) > 0 {
		normalizer = normalizers[0]
	}

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
		managed.NoBookCrossing = true
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
	defer book.mu.Unlock()

	for index, data := range payload.Data {
		symbolBook := book.manager.GetBook(data.Symbol)

		if symbolBook == nil {
			continue
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

				limitPrice := order.LimitPrice

				if book.normalizer != nil {
					limitPrice, err = book.normalizer.FormatPrice(data.Symbol, limitPrice)

					if err != nil {
						return errnie.Err(
							errnie.Validation,
							fmt.Sprintf("level3 price precision for %s", data.Symbol),
							err,
						)
					}
				}

				price := limitPrice.String()

				if level, ok := symbolSide.Levels[price]; ok && level == nil {
					delete(symbolSide.Levels, price)
				}

				if order.Event != "delete" {
					if order.OrderQty == nil {
						continue
					}

					orderQuantity := order.OrderQty

					if book.normalizer != nil {
						orderQuantity, err = book.normalizer.FormatSize(
							data.Symbol,
							orderQuantity,
						)

						if err != nil {
							return errnie.Err(
								errnie.Validation,
								fmt.Sprintf("level3 quantity precision for %s", data.Symbol),
								err,
							)
						}
					}

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
		if data.Checksum != 0 && !symbolBook.L3Checksum(strconv.FormatUint(
			uint64(data.Checksum),
			10,
		)).Match {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				fmt.Sprintf("level3 checksum mismatch for %s", data.Symbol),
				nil,
			))
		}

		if book.updates != nil {
			select {
			case book.updates <- data.Symbol:
			default:
			}
		}

	}

	return nil
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
