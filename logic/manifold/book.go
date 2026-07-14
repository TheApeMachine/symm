package manifold

import (
	"time"

	"github.com/krakenfx/api-go/v2/pkg/book"
)

/*
ObserveBook reads the executable boundary from the SDK book and reconciles only
the persistent carrier history needed by the field solver. Kraken remains the
sole owner of book ordering, depth, crossing, and touch calculation.
*/
func (slot *Slot) ObserveBook(managed *book.Book) ProcessResult {
	observation := ObservationMetadata{FrameType: "book", Count: 1}

	if managed == nil || managed.Name != slot.symbol {
		return slot.invalidate(observation, BookInvalid)
	}

	bestBid := managed.BestBid()
	bestAsk := managed.BestAsk()
	midpoint := managed.Midpoint()

	if bestBid == nil || bestAsk == nil || bestBid.Price == nil ||
		bestAsk.Price == nil || bestBid.Quantity == nil ||
		bestAsk.Quantity == nil || midpoint == nil {
		return slot.invalidate(observation, BookInvalid)
	}

	if bestBid.Price.Sign() <= 0 || bestAsk.Price.Cmp(bestBid.Price) <= 0 ||
		bestBid.Quantity.Sign() <= 0 || bestAsk.Quantity.Sign() <= 0 ||
		midpoint.Sign() <= 0 {
		return slot.invalidate(observation, NonPositiveMid)
	}

	if !slot.population.ReconcileBook(managed) {
		return ProcessResult{}
	}

	observation.At = slot.population.LastAt()

	if slot.advanceReady {
		observation.Count += slot.pending.metadata.Count
	}

	slot.pending = pendingObservation{
		metadata:        observation,
		bestBid:         bestBid.Price.Float64(),
		bestAsk:         bestAsk.Price.Float64(),
		bestBidQuantity: bestBid.Quantity.Float64(),
		bestAskQuantity: bestAsk.Quantity.Float64(),
		midPrice:        midpoint.Float64(),
	}
	slot.advanceReady = true

	return ProcessResult{
		Observation:  observation,
		Accounting:   slot.population.Accounting(),
		AdvanceReady: true,
	}
}

/*
ReconcileBook updates persistent solver carriers from the current SDK book.
The SDK already owns the complete visible population; this method preserves
only cross-observation identity and records what changed.
*/
func (population *Population) ReconcileBook(managed *book.Book) bool {
	initial := !population.initialized
	recovering := population.invalid != Valid

	if initial {
		population.orders = make(map[string]*PhysicalOrder)
		population.accounting = PopulationAccounting{}
		population.initialized = true
	}

	population.invalid = Valid
	seen := make(map[string]struct{}, len(population.orders))
	bidAt, bidChanged := population.reconcileSide(
		managed.Bids, OrderSideBid, seen, initial,
	)
	askAt, askChanged := population.reconcileSide(
		managed.Asks, OrderSideAsk, seen, initial,
	)
	at := bidAt

	if askAt.After(at) {
		at = askAt
	}

	if at.IsZero() || at.Before(population.lastAt) {
		at = population.lastAt
	}

	removed := population.removeAbsent(seen, at)
	changed := initial || recovering || bidChanged || askChanged || removed

	if !changed {
		return false
	}

	population.lastAt = at
	population.seedLifetime(at, initial)

	return true
}

/*
reconcileSide preserves carrier history while applying one SDK-owned side.
*/
func (population *Population) reconcileSide(
	managed *book.Side,
	direction OrderSide,
	seen map[string]struct{},
	initial bool,
) (time.Time, bool) {
	var at time.Time
	changed := false

	for _, level := range managed.Levels {
		if level.Timestamp.After(at) {
			at = level.Timestamp
		}

		for _, managedOrder := range level.Queue() {
			seen[managedOrder.ID] = struct{}{}

			if managedOrder.Timestamp.After(at) {
				at = managedOrder.Timestamp
			}

			price := managedOrder.LimitPrice.Float64()
			quantity := managedOrder.Quantity.Float64()
			existing := population.orders[managedOrder.ID]

			if existing == nil {
				population.orders[managedOrder.ID] = &PhysicalOrder{
					OrderID: managedOrder.ID, Side: direction,
					LimitPrice: price, Quantity: quantity,
					AddedAt: managedOrder.Timestamp, UpdatedAt: managedOrder.Timestamp,
				}

				if initial {
					population.accounting.recordInitial(quantity)
				} else {
					population.accounting.recordAdded(quantity)
				}

				changed = true
				continue
			}

			if existing.Side == direction && existing.LimitPrice == price &&
				existing.Quantity == quantity &&
				existing.UpdatedAt.Equal(managedOrder.Timestamp) {
				continue
			}

			population.accounting.recordAmended(existing.Quantity, quantity)
			existing.Side = direction
			existing.LimitPrice = price
			existing.Quantity = quantity
			existing.UpdatedAt = managedOrder.Timestamp
			changed = true
		}
	}

	return at, changed
}

/*
removeAbsent accounts for carriers no longer present in the complete SDK book
without inventing cancellation or execution semantics the book cannot prove.
*/
func (population *Population) removeAbsent(
	seen map[string]struct{},
	at time.Time,
) bool {
	changed := false

	for orderID, order := range population.orders {
		if _, exists := seen[orderID]; exists {
			continue
		}

		population.accounting.recordRemoved(order.Quantity)

		if population.lifetime != nil {
			population.lifetime.RecordCompleted(at.Sub(order.AddedAt))
		}

		delete(population.orders, orderID)
		changed = true
	}

	return changed
}

/*
seedLifetime right-censors the initial visible orders so age coordinates are
defined before the first observed removal completes an order lifetime.
*/
func (population *Population) seedLifetime(at time.Time, initial bool) {
	if !initial || population.lifetime == nil || population.lifetime.Ready() {
		return
	}

	for _, order := range population.orders {
		population.lifetime.Censor(at.Sub(order.AddedAt))
	}
}
