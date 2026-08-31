package broker

import (
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
liquidationBook is the bounded resident execution state for ONE symbol that has
an open execution lifecycle. It is owned by a Position, created lazily when L3
traffic first needs to price that position's liquidation, and released when the
position closes. It is deliberately not a general-purpose exchange book: it
retains only the bid chain needed to price the held SellableQty plus the best
ask needed to keep the book coherent, and its capacity is bounded by the venue
depth present in the feed.

It is the exact inverse of a market-wide BookOwner: no map[string]*SymbolBook,
no registry, no state for symbols without a position. The signal pipeline keeps
consuming raw L3 frames directly and never passes through this reducer.
*/
type liquidationBook struct {
	mu     sync.RWMutex
	symbol string
	bids   []kraken.BookOrder
	bidIdx map[string]int
	asks   []kraken.BookOrder
	askIdx map[string]int
	valid  bool
	saw    bool
}

func newLiquidationBook(symbol string) *liquidationBook {
	return &liquidationBook{
		symbol: symbol,
		bidIdx: make(map[string]int),
		askIdx: make(map[string]int),
	}
}

/*
Apply folds one L3 frame into the resident liquidation state. A snapshot (or
the first frame) replaces the sides; an update applies each add/modify/delete
exactly once in causal arrival order.
*/
func (book *liquidationBook) Apply(data kraken.Level3Data) {
	if book == nil || data.Symbol == "" {
		return
	}

	book.mu.Lock()
	defer book.mu.Unlock()

	if data.Type == "snapshot" || !book.saw {
		book.bids = book.bids[:0]
		book.asks = book.asks[:0]
		clear(book.bidIdx)
		clear(book.askIdx)

		for _, order := range data.Bids {
			if usableLiquidationOrder(order) {
				book.upsert(order, kraken.SideBid)
			}
		}

		for _, order := range data.Asks {
			if usableLiquidationOrder(order) {
				book.upsert(order, kraken.SideAsk)
			}
		}

		book.saw = true
		book.valid = book.coherent()

		return
	}

	for _, order := range data.Bids {
		book.applyOrder(order, kraken.SideBid)
	}

	for _, order := range data.Asks {
		book.applyOrder(order, kraken.SideAsk)
	}

	book.valid = book.coherent()
}

func usableLiquidationOrder(order kraken.Level3Order) bool {
	return order.OrderID != "" && order.LimitPrice != nil &&
		order.OrderQty != nil && order.LimitPrice.Sign() > 0 &&
		order.OrderQty.Sign() > 0
}

func (book *liquidationBook) applyOrder(order kraken.Level3Order, side kraken.Side) {
	if order.OrderID == "" {
		return
	}

	if order.Event == "delete" {
		book.remove(order.OrderID, side)

		return
	}

	if !usableLiquidationOrder(order) {
		return
	}

	book.upsert(order, side)
}

func (book *liquidationBook) upsert(order kraken.Level3Order, side kraken.Side) {
	resident := residentLiquidationOrder(order)

	if side == kraken.SideBid {
		if index, found := book.bidIdx[order.OrderID]; found {
			book.bids[index] = resident
			book.resortBids()

			return
		}

		book.bids = append(book.bids, resident)
		book.reindexBids()

		return
	}

	if index, found := book.askIdx[order.OrderID]; found {
		book.asks[index] = resident
		book.resortAsks()

		return
	}

	book.asks = append(book.asks, resident)
	book.reindexAsks()
}

func (book *liquidationBook) remove(orderID string, side kraken.Side) {
	if side == kraken.SideBid {
		index, found := book.bidIdx[orderID]

		if !found {
			return
		}

		book.bids = append(book.bids[:index], book.bids[index+1:]...)
		book.reindexBids()

		return
	}

	index, found := book.askIdx[orderID]

	if !found {
		return
	}

	book.asks = append(book.asks[:index], book.asks[index+1:]...)
	book.reindexAsks()
}

func (book *liquidationBook) reindexBids() {
	book.bidIdx = make(map[string]int, len(book.bids))
	insertionSortLiquidation(book.bids, kraken.SideBid)

	for index := range book.bids {
		book.bidIdx[book.bids[index].OrderID] = index
	}
}

func (book *liquidationBook) reindexAsks() {
	book.askIdx = make(map[string]int, len(book.asks))
	insertionSortLiquidation(book.asks, kraken.SideAsk)

	for index := range book.asks {
		book.askIdx[book.asks[index].OrderID] = index
	}
}

func (book *liquidationBook) resortBids() {
	insertionSortLiquidation(book.bids, kraken.SideBid)

	for index := range book.bids {
		book.bidIdx[book.bids[index].OrderID] = index
	}
}

func (book *liquidationBook) resortAsks() {
	insertionSortLiquidation(book.asks, kraken.SideAsk)

	for index := range book.asks {
		book.askIdx[book.asks[index].OrderID] = index
	}
}

func residentLiquidationOrder(order kraken.Level3Order) kraken.BookOrder {
	return kraken.BookOrder{
		OrderID:    order.OrderID,
		LimitPrice: decimal.NewFromInt64(0).Add(order.LimitPrice),
		OrderQty:   decimal.NewFromInt64(0).Add(order.OrderQty),
	}
}

func insertionSortLiquidation(orders []kraken.BookOrder, side kraken.Side) {
	for index := 0; index < len(orders); index++ {
		current := orders[index]
		position := index

		for position > 0 && liquidationOrderedBefore(orders[position-1], current, side) {
			orders[position] = orders[position-1]
			position--
		}

		orders[position] = current
	}
}

func liquidationOrderedBefore(left, right kraken.BookOrder, side kraken.Side) bool {
	if side == kraken.SideBid {
		return left.LimitPrice.Cmp(right.LimitPrice) < 0
	}

	return left.LimitPrice.Cmp(right.LimitPrice) > 0
}

/*
coherent reports whether the resident state is usable and non-crossed. It
requires a best bid and a best ask, ordered strictly below/above.
*/
func (book *liquidationBook) coherent() bool {
	if len(book.bids) == 0 || len(book.asks) == 0 {
		return false
	}

	bestBid := book.bids[0].LimitPrice
	bestAsk := book.asks[0].LimitPrice

	if bestBid == nil || bestAsk == nil {
		return false
	}

	return bestBid.Cmp(bestAsk) < 0
}

/*
Surface derives the exact executable-liquidation surface for the held
SellableQty from the resident bid chain, under the read lock. It returns a
surface with BookComplete/FullExecutable set truthfully; it never synthesizes a
VWAP for insufficient depth and never falls back to ticker.
*/
func (book *liquidationBook) Surface(
	sellableQty *decimal.Decimal,
	floor *decimal.Decimal,
	fee *kraken.TradeVolumeFee,
	at time.Time,
) *types.ExecutionSurface {
	surface := &types.ExecutionSurface{
		Symbol:      book.symbol,
		At:          at,
		SellableQty: decimal.NewFromInt64(0).Add(sellableQty),
	}

	if sellableQty == nil || sellableQty.Sign() <= 0 ||
		fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 ||
		fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
		return surface
	}

	book.mu.RLock()
	defer book.mu.RUnlock()

	if !book.valid || len(book.bids) == 0 {
		return surface
	}

	surface.BookComplete = true

	if book.bids[0].LimitPrice != nil {
		surface.BestBid = decimal.NewFromInt64(0).Add(book.bids[0].LimitPrice)
	}

	executableQty := decimal.NewFromInt64(0)
	floorCoverageQty := decimal.NewFromInt64(0)
	grossProceeds := decimal.NewFromInt64(0)
	remaining := decimal.NewFromInt64(0).Add(sellableQty)

	for _, order := range book.bids {
		if order.LimitPrice == nil || order.OrderQty == nil ||
			order.LimitPrice.Sign() <= 0 || order.OrderQty.Sign() <= 0 {
			continue
		}

		executableQty = executableQty.Add(order.OrderQty)

		if floor != nil && order.LimitPrice.Cmp(floor) >= 0 {
			floorCoverageQty = floorCoverageQty.Add(order.OrderQty)
		}

		if remaining.Sign() <= 0 {
			continue
		}

		fill := order.OrderQty

		if remaining.Cmp(fill) < 0 {
			fill = remaining
		}

		grossProceeds = grossProceeds.Add(order.LimitPrice.Mul(fill))
		remaining = remaining.Sub(fill)
	}

	surface.ExecutableQty = executableQty
	surface.FloorCoverageQty = floorCoverageQty

	if remaining.Sign() > 0 || executableQty.Cmp(sellableQty) < 0 {
		return surface
	}

	feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(
		decimal.NewFromInt64(100),
	)
	surface.FullyExecutable = true
	surface.ExecutableVWAP = grossProceeds.Div(sellableQty)
	surface.ExecutableValue = grossProceeds.Sub(grossProceeds.Mul(feeRate))

	return surface
}
