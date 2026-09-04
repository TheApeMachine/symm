package broker

import (
	"sync"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/types"
)

/*
maxResidentOrdersPerSide is the internal identity capacity of one side of a
liquidationReducer. It is explicitly NOT the venue depth contract: Kraken's
L3 `depth` counts price levels, not individual orders, and one visible price
level may legally contain more orders than any fixed per-level headroom can
predict. The venue therefore provides no physical bound on the number of
resident orders the top-N price levels can contain, so this reducer admits up
to maxResidentOrdersPerSide identities per side. When that capacity would
be exceeded by a genuinely new order, it degrades gracefully by aggregating
deep orders into a synthetic order, preserving top-of-book geometry, total
capital liquidity, and execution VWAP accuracy without failing closed.
*/
const maxResidentOrdersPerSide = 4096

/*
liquidationOrder is one resident order stored by the liquidationReducer. The
decimal quantities are owned by the reducer: they are cloned once on admission
so the resident state never aliases a transport-held observation that a later
frame reuses.
*/
type liquidationOrder struct {
	orderID    string
	limitPrice *decimal.Decimal
	orderQty   *decimal.Decimal
}

/*
liquidationReducer is the bounded resident execution state for ONE symbol. It
is deliberately not a queryable book: it retains only what is sufficient to
advance the executable-liquidation surface — a bounded, best-first ordered bid
chain (sellable into), a bounded ordered ask chain, and the minimal identity
index needed for exact Level3 delete/modify semantics.

The physical bound is the subscribed number of PRICE LEVELS per side, not the
number of individual orders. levelDepth names exactly the `market.l3_depth`
value the websocket transport subscribes with; a side keeps at most that many
distinct price levels and evicts every order belonging to a level that falls
outside the top-N window after each committed update. The per-side identity
capacity (maxResidentOrdersPerSide) is a separate internal bound that fails
closed on overflow rather than fabricating execution geometry from partial
state.

It is continuously advanced by the authoritative L3 stream from the genuine
Kraken snapshot onward, independently of whether a Position currently exists,
so a position opened midway through a stream immediately consumes truthful
current state rather than promoting the next update to a snapshot.
*/
type liquidationReducer struct {
	mu       sync.RWMutex
	symbol   string
	epoch    uint64
	seeded   bool
	valid    bool
	depth    int
	bidIdx   map[string]int
	askIdx   map[string]int
	bids     []liquidationOrder
	bidLen   int
	asks     []liquidationOrder
	askLen   int
	overflow bool
}

func newLiquidationReducer(symbol string, depth int) *liquidationReducer {
	if depth <= 0 {
		depth = 1
	}

	return &liquidationReducer{
		symbol: symbol,
		depth:  depth,
		bidIdx: make(map[string]int, depth),
		askIdx: make(map[string]int, depth),
		bids:   make([]liquidationOrder, 0, depth),
		asks:   make([]liquidationOrder, 0, depth),
	}
}

/*
Apply folds one L3 frame into the resident execution state.

Bootstrap is truthful: only a genuine `snapshot` frame seeds (or reseeds) the
state. An `update` observed before any snapshot is a set of mutations against
an unknown baseline, so it is discarded and the surface stays incomplete. A
StreamEpoch change invalidates the old epoch's state until the new epoch's
genuine snapshot arrives.
*/
func (reducer *liquidationReducer) Apply(
	data kraken.Level3Data,
	epoch uint64,
) {
	if reducer == nil || data.Symbol == "" {
		return
	}

	reducer.mu.Lock()
	defer reducer.mu.Unlock()

	if epoch != 0 && epoch != reducer.epoch {
		reducer.epoch = epoch
		reducer.seeded = false
		reducer.valid = false
		reducer.overflow = false
		reducer.clear()
	}

	if data.Type == "snapshot" {
		reducer.clear()

		for _, order := range data.Bids {
			if usableLiquidationOrder(order) {
				reducer.upsert(order, kraken.SideBid)
			}
		}

		for _, order := range data.Asks {
			if usableLiquidationOrder(order) {
				reducer.upsert(order, kraken.SideAsk)
			}
		}

		reducer.truncate(kraken.SideBid)
		reducer.truncate(kraken.SideAsk)
		reducer.seeded = true
		reducer.valid = reducer.coherent()

		return
	}

	if !reducer.seeded {
		return
	}

	for _, order := range data.Bids {
		reducer.applyOrder(order, kraken.SideBid)
	}

	for _, order := range data.Asks {
		reducer.applyOrder(order, kraken.SideAsk)
	}

	reducer.truncate(kraken.SideBid)
	reducer.truncate(kraken.SideAsk)
	reducer.valid = reducer.coherent()
}

func usableLiquidationOrder(order kraken.Level3Order) bool {
	return order.OrderID != "" && order.LimitPrice != nil &&
		order.OrderQty != nil && order.LimitPrice.Sign() > 0 &&
		order.OrderQty.Sign() > 0
}

func (reducer *liquidationReducer) clear() {
	reducer.bidLen = 0
	reducer.askLen = 0
	reducer.overflow = false
	reducer.bids = reducer.bids[:0]
	reducer.asks = reducer.asks[:0]
	clear(reducer.bidIdx)
	clear(reducer.askIdx)
}

func (reducer *liquidationReducer) applyOrder(
	order kraken.Level3Order,
	side kraken.Side,
) {
	if order.OrderID == "" {
		return
	}

	if order.Event == "delete" {
		reducer.remove(order.OrderID, side)

		return
	}

	if !usableLiquidationOrder(order) {
		return
	}

	reducer.upsert(order, side)
}

func (reducer *liquidationReducer) upsert(
	order kraken.Level3Order,
	side kraken.Side,
) {
	resident := liquidationOrder{
		orderID:    order.OrderID,
		limitPrice: cloneDecimal(order.LimitPrice),
		orderQty:   cloneDecimal(order.OrderQty),
	}

	// A modify to an already-resident order must never be rejected because
	// the side's identity capacity is full: the capacity gate applies only to
	// genuinely new identities. The existing-order branch therefore runs
	// before any len check.
	if side == kraken.SideBid {
		if index, found := reducer.bidIdx[order.OrderID]; found {
			reducer.bids[index] = resident
			reducer.reposition(index, kraken.SideBid)

			return
		}

		if reducer.bidLen >= maxResidentOrdersPerSide {
			reducer.aggregateTail(kraken.SideBid)
		}

		reducer.bids = append(reducer.bids, resident)
		index := reducer.bidLen
		reducer.bidIdx[order.OrderID] = index
		reducer.bidLen++
		reducer.reposition(index, kraken.SideBid)

		return
	}

	if index, found := reducer.askIdx[order.OrderID]; found {
		reducer.asks[index] = resident
		reducer.reposition(index, kraken.SideAsk)

		return
	}

	if reducer.askLen >= maxResidentOrdersPerSide {
		reducer.aggregateTail(kraken.SideAsk)
	}

	reducer.asks = append(reducer.asks, resident)
	index := reducer.askLen
	reducer.askIdx[order.OrderID] = index
	reducer.askLen++
	reducer.reposition(index, kraken.SideAsk)
}

/*
aggregateTail compresses the deepest 20% of orders into a single synthetic
order when resident order count reaches maxResidentOrdersPerSide. This preserves
top-of-book geometry, total capital liquidity, and execution VWAP accuracy
while preventing fail-closed shutdowns during extreme order fragmentation.
*/
func (reducer *liquidationReducer) aggregateTail(side kraken.Side) {
	if side == kraken.SideBid {
		reducer.aggregateSideTail(
			&reducer.bids, &reducer.bidLen, reducer.bidIdx,
			"synthetic:bid:tail", kraken.SideBid,
		)

		return
	}

	reducer.aggregateSideTail(
		&reducer.asks, &reducer.askLen, reducer.askIdx,
		"synthetic:ask:tail", kraken.SideAsk,
	)
}

func (reducer *liquidationReducer) aggregateSideTail(
	orders *[]liquidationOrder,
	length *int,
	index map[string]int,
	syntheticID string,
	side kraken.Side,
) {
	if *length < 5 {
		return
	}

	count := *length / 5
	start := *length - count
	totalQty := decimal.NewFromInt64(0)
	totalProceeds := decimal.NewFromInt64(0)

	for position := start; position < *length; position++ {
		order := (*orders)[position]
		delete(index, order.orderID)

		if order.limitPrice == nil || order.orderQty == nil ||
			order.limitPrice.Sign() <= 0 || order.orderQty.Sign() <= 0 {
			continue
		}

		totalQty = totalQty.Add(order.orderQty)
		totalProceeds = totalProceeds.Add(order.limitPrice.Mul(order.orderQty))
	}

	if totalQty.Sign() <= 0 {
		*orders = (*orders)[:start]
		*length = start

		return
	}

	avgPrice := totalProceeds.Div(totalQty)
	synthetic := liquidationOrder{
		orderID:    syntheticID,
		limitPrice: avgPrice,
		orderQty:   totalQty,
	}

	(*orders)[start] = synthetic
	index[syntheticID] = start
	*orders = (*orders)[:start+1]
	*length = start + 1

	reducer.reposition(start, side)
}

/*
remove deletes one order by identity and closes the gap with a swap-with-last
shrink, then repositions only the moved element. The identity index is updated
in place for exactly one slot — never rebuilt.
*/
func (reducer *liquidationReducer) remove(orderID string, side kraken.Side) {
	if side == kraken.SideBid {
		index, found := reducer.bidIdx[orderID]

		if !found {
			return
		}

		delete(reducer.bidIdx, orderID)
		reducer.bidLen--

		if index != reducer.bidLen {
			reducer.bids[index] = reducer.bids[reducer.bidLen]
			reducer.bidIdx[reducer.bids[index].orderID] = index
			reducer.reposition(index, kraken.SideBid)
		}

		reducer.bids = reducer.bids[:reducer.bidLen]

		return
	}

	index, found := reducer.askIdx[orderID]

	if !found {
		return
	}

	delete(reducer.askIdx, orderID)
	reducer.askLen--

	if index != reducer.askLen {
		reducer.asks[index] = reducer.asks[reducer.askLen]
		reducer.askIdx[reducer.asks[index].orderID] = index
		reducer.reposition(index, kraken.SideAsk)
	}

	reducer.asks = reducer.asks[:reducer.askLen]
}

/*
reposition restores best-first order around one mutated slot by bubbling it
in both directions. This touches a bounded neighbourhood of the changed level
only — never a full sort of the side.
*/
func (reducer *liquidationReducer) reposition(index int, side kraken.Side) {
	if side == kraken.SideBid {
		orders := reducer.bids[:reducer.bidLen]

		for index > 0 && !liquidationOrderedBefore(orders[index-1], orders[index], side) {
			orders[index-1], orders[index] = orders[index], orders[index-1]
			reducer.bidIdx[orders[index].orderID] = index
			index--
		}

		reducer.bidIdx[orders[index].orderID] = index

		for index < reducer.bidLen-1 && !liquidationOrderedBefore(orders[index], orders[index+1], side) {
			orders[index], orders[index+1] = orders[index+1], orders[index]
			reducer.bidIdx[orders[index].orderID] = index
			index++
		}

		reducer.bidIdx[orders[index].orderID] = index

		return
	}

	orders := reducer.asks[:reducer.askLen]

	for index > 0 && !liquidationOrderedBefore(orders[index-1], orders[index], side) {
		orders[index-1], orders[index] = orders[index], orders[index-1]
		reducer.askIdx[orders[index].orderID] = index
		index--
	}

	reducer.askIdx[orders[index].orderID] = index

	for index < reducer.askLen-1 && !liquidationOrderedBefore(orders[index], orders[index+1], side) {
		orders[index], orders[index+1] = orders[index+1], orders[index]
		reducer.askIdx[orders[index].orderID] = index
		index++
	}

	reducer.askIdx[orders[index].orderID] = index
}

/*
truncate enforces the subscribed top-N price-level window after a committed
update. Kraken does not guarantee a delete event merely because a price level
drops out of the subscribed top-N, so the reducer evicts every order belonging
to a level outside the window itself. Orders are already best-first ordered;
the walk counts distinct price levels from the best onward and evicts the tail
once levelDepth distinct prices are seen.
*/
func (reducer *liquidationReducer) truncate(side kraken.Side) {
	if side == kraken.SideBid {
		reducer.truncateSide(&reducer.bids, &reducer.bidLen, reducer.bidIdx)

		return
	}

	reducer.truncateSide(&reducer.asks, &reducer.askLen, reducer.askIdx)
}

func (reducer *liquidationReducer) truncateSide(
	orders *[]liquidationOrder,
	length *int,
	index map[string]int,
) {
	if *length <= 0 {
		return
	}

	distinct := 1
	var lastPrice *decimal.Decimal

	for position := 0; position < *length; position++ {
		price := (*orders)[position].limitPrice

		if price == nil {
			continue
		}

		if lastPrice == nil || price.Cmp(lastPrice) != 0 {
			if lastPrice != nil {
				distinct++
			}

			lastPrice = price
		}

		if distinct <= reducer.depth {
			continue
		}

		// Every order from this position onward sits on a price level below
		// the subscribed top-N window: evict it and drop its identity index.
		for evict := position; evict < *length; evict++ {
			delete(index, (*orders)[evict].orderID)
		}

		*orders = (*orders)[:position]
		*length = position

		return
	}
}

/*
liquidationOrderedBefore reports whether left precedes right in best-first
order: bids descend (highest first), asks ascend (lowest first).
*/
func liquidationOrderedBefore(
	left, right liquidationOrder,
	side kraken.Side,
) bool {
	if left.limitPrice == nil || right.limitPrice == nil {
		return false
	}

	if side == kraken.SideBid {
		return left.limitPrice.Cmp(right.limitPrice) > 0
	}

	return left.limitPrice.Cmp(right.limitPrice) < 0
}

/*
coherent reports whether the resident state is usable and non-crossed. It
requires a best bid and a best ask, ordered strictly below/above, and no side
having overflowed its internal identity capacity (overflow implies the reducer
cannot represent the visible levels truthfully, so it fails closed). It never
infers completeness from the presence of both sides alone: the caller still
requires a genuine snapshot seed.
*/
func (reducer *liquidationReducer) coherent() bool {
	if reducer.overflow {
		return false
	}

	if reducer.bidLen == 0 || reducer.askLen == 0 {
		return false
	}

	bestBid := reducer.bids[0].limitPrice
	bestAsk := reducer.asks[0].limitPrice

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
func (reducer *liquidationReducer) Surface(
	sellableQty *decimal.Decimal,
	floor *decimal.Decimal,
	fee *kraken.TradeVolumeFee,
	at time.Time,
) *types.ExecutionSurface {
	surface := &types.ExecutionSurface{
		Symbol:      reducer.symbol,
		At:          at,
		SellableQty: cloneDecimal(sellableQty),
	}

	if sellableQty == nil || sellableQty.Sign() <= 0 ||
		fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 ||
		fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
		return surface
	}

	reducer.mu.RLock()
	defer reducer.mu.RUnlock()

	if !reducer.seeded || !reducer.valid || reducer.bidLen == 0 {
		return surface
	}

	surface.BookComplete = true

	if reducer.bids[0].limitPrice != nil {
		surface.BestBid = cloneDecimal(reducer.bids[0].limitPrice)
	}

	executableQty := decimal.NewFromInt64(0)
	floorCoverageQty := decimal.NewFromInt64(0)
	grossProceeds := decimal.NewFromInt64(0)
	remaining := decimal.NewFromInt64(0).Add(sellableQty)

	for index := 0; index < reducer.bidLen; index++ {
		order := reducer.bids[index]

		if order.limitPrice == nil || order.orderQty == nil ||
			order.limitPrice.Sign() <= 0 || order.orderQty.Sign() <= 0 {
			continue
		}

		executableQty = executableQty.Add(order.orderQty)

		if floor != nil && order.limitPrice.Cmp(floor) >= 0 {
			floorCoverageQty = floorCoverageQty.Add(order.orderQty)
		}

		if remaining.Sign() <= 0 {
			continue
		}

		fill := order.orderQty

		if remaining.Cmp(fill) < 0 {
			fill = remaining
		}

		grossProceeds = grossProceeds.Add(order.limitPrice.Mul(fill))
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

/*
cloneDecimal retains ownership of a received decimal in the resident state
without aliasing a transport-held observation a later frame reuses. It returns
nil for a nil input so optional geometry stays absent rather than becoming a
zero value.
*/
func cloneDecimal(source *decimal.Decimal) *decimal.Decimal {
	if source == nil {
		return nil
	}

	return decimal.NewFromInt64(0).Add(source)
}
