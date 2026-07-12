package manifold

import (
	"sort"
	"strings"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
)

/*
InvalidReason reports why a symbol cannot advance the field.
*/
type InvalidReason string

const (
	Valid            InvalidReason = ""
	ChecksumFailed   InvalidReason = "checksum_failed"
	SequenceGap      InvalidReason = "sequence_gap"
	UnknownEvent     InvalidReason = "unknown_event"
	MissingOrder     InvalidReason = "missing_order"
	DuplicateOrder   InvalidReason = "duplicate_order"
	NonPositiveMid   InvalidReason = "non_positive_mid"
	StaleSnapshot    InvalidReason = "stale_snapshot"
	UnmappedCarriers InvalidReason = "unmapped_carriers"
	SolverFailed     InvalidReason = "solver_failed"
	BookInvalid      InvalidReason = "book_invalid"
	TimestampRegress InvalidReason = "timestamp_regress"
	StabilityFailed  InvalidReason = "stability_failed"
)

/*
Level3Book validates checksum continuity and exposes the merged top of book.
*/
type Level3Book interface {
	Apply(row kraken.Level3Data, pricePrecision, qtyPrecision int) bool
	TopOfBook(symbol string) (bid, ask float64, ok bool)
	InvalidReason(symbol string) InvalidReason
}

/*
Population owns the per-symbol order registry and exact quantity accounting.
*/
type Population struct {
	symbol      string
	depth       int
	orders      map[string]*PhysicalOrder
	accounting  PopulationAccounting
	lifetime    *LifetimeEstimator
	priceLevels []float64
	seenPrices  map[float64]struct{}
	epoch       uint64
	invalid     InvalidReason
	lastAt      time.Time
}

func NewPopulation(symbol string, lifetime *LifetimeEstimator, depth int) *Population {
	return &Population{
		symbol:      symbol,
		depth:       depth,
		orders:      map[string]*PhysicalOrder{},
		lifetime:    lifetime,
		priceLevels: make([]float64, 0, depth+1),
		seenPrices:  make(map[float64]struct{}, depth+1),
	}
}

func (population *Population) Symbol() string {
	return population.symbol
}

func (population *Population) Epoch() uint64 {
	return population.epoch
}

/*
BeginEpoch freezes the latest fully applied population for one field advance.
Multiple L3 rows may be coalesced before this boundary, but every produced
manifold state receives exactly one new population epoch.
*/
func (population *Population) BeginEpoch() uint64 {
	population.epoch++
	return population.epoch
}

func (population *Population) InvalidReason() InvalidReason {
	return population.invalid
}

func (population *Population) LastAt() time.Time {
	return population.lastAt
}

func (population *Population) Ready() bool {
	return population.invalid == ""
}

func (population *Population) Accounting() PopulationAccounting {
	return population.accounting
}

func (population *Population) Orders() []*PhysicalOrder {
	orderIDs := make([]string, 0, len(population.orders))

	for orderID := range population.orders {
		orderIDs = append(orderIDs, orderID)
	}

	sort.Strings(orderIDs)

	orders := make([]*PhysicalOrder, 0, len(orderIDs))

	for _, orderID := range orderIDs {
		orders = append(orders, population.orders[orderID])
	}

	return orders
}

func (population *Population) Apply(row kraken.Level3Data, midPrice float64) {
	snapshot := strings.EqualFold(row.Type, "snapshot")

	if snapshot {
		population.resetSnapshot(row.Timestamp)
	}

	if population.invalid != "" {
		return
	}

	if midPrice <= 0 {
		population.invalidate(NonPositiveMid)
		return
	}

	if !row.Timestamp.IsZero() {
		population.lastAt = row.Timestamp
	}

	valid := population.applySide(row.Bids, OrderSideBid, row.Type) &&
		population.applySide(row.Asks, OrderSideAsk, row.Type)

	if !valid {
		population.invalidate(MissingOrder)
		return
	}

	population.truncate()

	if snapshot && population.lifetime != nil &&
		!population.lifetime.Ready() {
		for _, order := range population.orders {
			population.lifetime.Censor(row.Timestamp.Sub(order.AddedAt))
		}
	}

}

/*
truncate mirrors Kraken's subscribed price-level boundary after a complete row.
Orders leaving visibility are not cancellations or fills; ScopedOut records the
quantity boundary needed to reconcile the visible population ledger.
*/
func (population *Population) truncate() {
	population.truncateSide(OrderSideBid)
	population.truncateSide(OrderSideAsk)
}

func (population *Population) truncateSide(side OrderSide) {
	prices := population.priceLevels[:0]
	clear(population.seenPrices)

	for _, order := range population.orders {
		if order.Side != side {
			continue
		}

		if _, exists := population.seenPrices[order.LimitPrice]; exists {
			continue
		}

		population.seenPrices[order.LimitPrice] = struct{}{}
		prices = append(prices, order.LimitPrice)
	}

	population.priceLevels = prices

	if population.depth <= 0 || len(prices) <= population.depth {
		return
	}

	sort.Float64s(prices)
	boundary := prices[population.depth-1]

	if side == OrderSideBid {
		boundary = prices[len(prices)-population.depth]
	}

	for orderID, order := range population.orders {
		outside := side == OrderSideBid && order.Side == side && order.LimitPrice < boundary
		outside = outside || side == OrderSideAsk && order.Side == side && order.LimitPrice > boundary

		if !outside {
			continue
		}

		population.accounting.recordScopedOut(order.Quantity)
		delete(population.orders, orderID)
	}
}

func (population *Population) resetSnapshot(at time.Time) {
	if population.lifetime != nil && !at.IsZero() {
		for _, order := range population.orders {
			population.lifetime.Censor(at.Sub(order.AddedAt))
		}
	}

	population.orders = map[string]*PhysicalOrder{}
	population.accounting = PopulationAccounting{}
	population.invalid = ""
}

func (population *Population) applySide(
	orders []kraken.Level3Order,
	side OrderSide,
	frameType string,
) bool {
	if strings.EqualFold(frameType, "snapshot") {
		for _, wire := range orders {
			if !population.insert(side, wire, wire.OrderQty) {
				return false
			}

			population.accounting.recordInitial(wire.OrderQty)
		}

		return true
	}

	for _, wire := range orders {
		if !population.applyEvent(side, wire) {
			return false
		}
	}

	return true
}

func (population *Population) applyEvent(side OrderSide, wire kraken.Level3Order) bool {
	switch wire.Event {
	case "add":
		if _, exists := population.orders[wire.OrderID]; exists {
			population.invalidate(DuplicateOrder)
			return false
		}

		if !population.insert(side, wire, wire.OrderQty) {
			return false
		}

		population.accounting.recordAdded(wire.OrderQty)
		return true
	case "modify":
		existing, ok := population.orders[wire.OrderID]

		if !ok {
			return false
		}

		population.accounting.recordAmended(existing.Quantity, wire.OrderQty)
		existing.LimitPrice = wire.LimitPrice
		existing.Quantity = wire.OrderQty
		existing.UpdatedAt = wire.Timestamp
		population.orders[wire.OrderID] = existing

		return true
	case "delete":
		existing, ok := population.orders[wire.OrderID]

		if !ok {
			return false
		}

		if population.lifetime != nil {
			at := wire.Timestamp

			if at.IsZero() {
				at = population.lastAt
			}

			population.lifetime.RecordCompleted(at.Sub(existing.AddedAt))
		}

		population.accounting.recordCancelled(existing.Quantity)
		delete(population.orders, wire.OrderID)

		return true
	default:
		population.invalidate(UnknownEvent)

		errnie.Error(errnie.Err(
			errnie.Validation,
			"logic manifold population: unknown level3 event",
			nil,
		))

		return false
	}
}

func (population *Population) insert(side OrderSide, wire kraken.Level3Order, quantity float64) bool {
	if _, exists := population.orders[wire.OrderID]; exists {
		population.invalidate(DuplicateOrder)
		return false
	}

	at := wire.Timestamp

	if at.IsZero() {
		at = population.lastAt
	}

	population.orders[wire.OrderID] = &PhysicalOrder{
		OrderID:    wire.OrderID,
		Side:       side,
		LimitPrice: wire.LimitPrice,
		Quantity:   quantity,
		AddedAt:    at,
		UpdatedAt:  at,
	}

	return true
}

func (population *Population) invalidate(reason InvalidReason) {
	population.invalid = reason
}
