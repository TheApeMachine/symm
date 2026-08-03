package websocket

import (
	"context"
	"fmt"
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
	ctx      context.Context
	cancel   context.CancelFunc
	status   types.Status
	statusMu sync.RWMutex
	mu       sync.RWMutex
	manager  *spot.BookManager
}

func NewBook(ctx context.Context) *Book {
	errnie.Info("websocket: initializing book manager")
	ctx, cancel := context.WithCancel(ctx)

	book := &Book{
		ctx:     ctx,
		cancel:  cancel,
		status:  types.INITIALIZING,
		manager: spot.NewBookManager(),
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

func (book *Book) Get(symbol string) *spotbook.Book {
	book.mu.Lock()
	defer book.mu.Unlock()

	return cloneBook(book.manager.GetBook(symbol))
}

func (book *Book) Create(symbol string, depth int) {
	book.mu.Lock()
	defer book.mu.Unlock()

	book.manager.CreateBook(symbol, depth)
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

				price := order.LimitPrice.String()

				if level, ok := symbolSide.Levels[price]; ok && level == nil {
					delete(symbolSide.Levels, price)
				}

				if order.Event != "delete" {
					if order.OrderQty == nil {
						continue
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

				if symbolSide.Levels[price] == nil {
					continue
				}

				symbolBook.Update(&spotbook.UpdateOptions{
					Direction: direction,
					ID:        order.OrderID,
					Price:     order.LimitPrice,
					Quantity:  decimal.NewFromInt64(0),
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
	book.mu.Lock()
	defer book.mu.Unlock()

	out := &sync.Map{}

	for _, symbol := range book.manager.GetBooks() {
		out.Store(symbol, cloneBook(book.manager.GetBook(symbol)))
	}

	return out
}

func cloneBook(source *spotbook.Book) *spotbook.Book {
	if source == nil {
		return nil
	}

	cloned := spotbook.New()
	cloned.Name = source.Name
	cloned.MaxDepth = source.MaxDepth
	cloned.NoBookCrossing = false
	cloned.EnableMaxDepth = false
	cloneSide(cloned, source.Bids, spotbook.Bid)
	cloneSide(cloned, source.Asks, spotbook.Ask)
	primeQueues(cloned)
	cloned.NoBookCrossing = source.NoBookCrossing
	cloned.EnableMaxDepth = source.EnableMaxDepth

	return cloned
}

func primeQueues(book *spotbook.Book) {
	for _, side := range []*spotbook.Side{book.Bids, book.Asks} {
		for _, level := range side.Levels {
			level.Queue()
		}
	}
}

func cloneSide(
	destination *spotbook.Book,
	source *spotbook.Side,
	direction spotbook.BookDirection,
) {
	if source == nil {
		return
	}

	for _, level := range source.Levels {
		if level == nil || level.Price == nil || level.Quantity == nil {
			continue
		}

		orders := level.Queue()

		if len(orders) == 0 {
			destination.Update(&spotbook.UpdateOptions{
				Direction: direction,
				Price:     level.Price,
				Quantity:  level.Quantity,
				Timestamp: level.Timestamp,
				Silent:    true,
			})

			continue
		}

		for _, order := range orders {
			if order == nil || order.LimitPrice == nil || order.Quantity == nil {
				continue
			}

			destination.Update(&spotbook.UpdateOptions{
				Direction: direction,
				ID:        order.ID,
				Price:     order.LimitPrice,
				Quantity:  order.Quantity,
				Timestamp: order.Timestamp,
				Silent:    true,
			})
		}
	}
}
