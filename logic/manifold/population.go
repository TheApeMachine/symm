package manifold

import (
	"sort"
	"time"
)

/*
InvalidReason reports why a symbol cannot advance the field.
*/
type InvalidReason string

const (
	Valid            InvalidReason = ""
	NonPositiveMid   InvalidReason = "non_positive_mid"
	UnmappedCarriers InvalidReason = "unmapped_carriers"
	SolverFailed     InvalidReason = "solver_failed"
	BookInvalid      InvalidReason = "book_invalid"
	TimestampRegress InvalidReason = "timestamp_regress"
	StabilityFailed  InvalidReason = "stability_failed"
)

/*
Population owns the persistent solver carriers derived from one SDK book. It
retains only state the SDK book does not provide: prior coordinates, lifetime
history, and quantity accounting across observations.
*/
type Population struct {
	symbol      string
	initialized bool
	orders      map[string]*PhysicalOrder
	accounting  PopulationAccounting
	lifetime    *LifetimeEstimator
	epoch       uint64
	invalid     InvalidReason
	lastAt      time.Time
}

/*
NewPopulation creates the carrier population for one market symbol.
*/
func NewPopulation(symbol string, lifetime *LifetimeEstimator) *Population {
	return &Population{
		symbol:   symbol,
		orders:   make(map[string]*PhysicalOrder),
		lifetime: lifetime,
	}
}

/*
Symbol returns the market owned by this population.
*/
func (population *Population) Symbol() string {
	return population.symbol
}

/*
Epoch returns the last field epoch begun from this population.
*/
func (population *Population) Epoch() uint64 {
	return population.epoch
}

/*
BeginEpoch identifies one immutable field advance from the current carriers.
*/
func (population *Population) BeginEpoch() uint64 {
	population.epoch++

	return population.epoch
}

/*
InvalidReason returns the current reason field advancement is blocked.
*/
func (population *Population) InvalidReason() InvalidReason {
	return population.invalid
}

/*
LastAt returns the latest event time retained from the SDK book.
*/
func (population *Population) LastAt() time.Time {
	return population.lastAt
}

/*
Ready reports whether the current carriers are valid for field advancement.
*/
func (population *Population) Ready() bool {
	return population.invalid == Valid
}

/*
Accounting returns the exact visible-quantity ledger for the current carriers.
*/
func (population *Population) Accounting() PopulationAccounting {
	return population.accounting
}

/*
Orders returns carriers in stable identity order so repeated field advances are
deterministic even though the SDK stores price levels in maps.
*/
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

/*
invalidate prevents field advancement until a valid SDK book is observed.
*/
func (population *Population) invalidate(reason InvalidReason) {
	population.invalid = reason
}
