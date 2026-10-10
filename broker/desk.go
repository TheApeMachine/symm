package broker

import (
	"context"
	"fmt"
	"math"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bytedance/sonic"
	"github.com/google/uuid"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/nomagique/core"
	"github.com/theapemachine/symm/nomagique/runtime"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
)

/*
PositionState is the lifecycle phase of one symbol's position on the Desk.
*/
type PositionState int

const (
	FLAT PositionState = iota
	ENTERING
	HOLDING
	EXITING
)

/*
Sell triggers. The learned exit (the S3 prefix match) is the primary exit and
always sells everything still open. The capacity monitor is a secondary risk
mechanism that only reduces exposure: it trims the position to what the bids
absorb within the slippage budget, and exits fully when that is below the
instrument minimum. A manual exit comes from the UI.
*/
const (
	TriggerLearnedExit  = "learned_exit"
	TriggerCapacityTrim = "capacity_trim"
	TriggerCapacityExit = "capacity_exit"
	TriggerManual       = "manual"
)

/*
Sell is one sell decision: which trigger fired, how much it sold, and the
exit capacity snapshot it read. Ratio is capacity over the open quantity at
the decision, 0 when the capacity was undefined. A trigger that fired while
the position was already exiting is recorded with a nil Quantity and a Note,
so both triggers stay visible.
*/
type Sell struct {
	At       time.Time
	Trigger  string
	Quantity *decimal.Decimal
	Capacity Capacity
	Ratio    float64
	Budget   float64
	Note     string
}

/*
ShadowLedger is the evaluation-truth accounting of a position: every venue
fill walked through the as-of book (Shadow). Short is venue-filled quantity
beyond the displayed depth; Unpriced counts venue fills no verified book
version covered, which leaves the ledger undefined. Base quantity the shadow
could not sell (Bought - Sold at close) is valued at zero.
*/
type ShadowLedger struct {
	Cost     float64
	Proceeds float64
	Fees     float64
	Bought   float64
	Sold     float64
	Short    float64
	Unpriced int
}

/*
Defined reports whether every venue fill had an as-of book to walk.
*/
func (ledger ShadowLedger) Defined() bool {
	return ledger.Unpriced == 0
}

/*
Realized is shadow proceeds minus shadow cost and fees.
*/
func (ledger ShadowLedger) Realized() float64 {
	return ledger.Proceeds - ledger.Cost - ledger.Fees
}

/*
Closure is the realized outcome of one completed round trip, built from the
venue's own fill costs and fees, with the shadow ledger beside it and every
sell decision that closed it.
*/
type Closure struct {
	Symbol   string
	Cost     *decimal.Decimal // Cumulative buy cost.
	Proceeds *decimal.Decimal // Cumulative sell proceeds.
	Fees     *decimal.Decimal // Cumulative fees across both legs.
	Realized *decimal.Decimal // Proceeds - Cost - Fees.
	Sells    []Sell
	Shadow   ShadowLedger
	ClosedAt time.Time
}

/*
order is one submitted order.
*/
type order struct {
	side     Direction
	quantity *decimal.Decimal
	trigger  string
}

/*
entryPlan is the sized entry worked in child market orders. target is the
total quantity; each child takes at most the ask capacity within the budget
of the newest book version (after shadow consumption), and the next child
waits for the previous fill and a newer version. The plan ends at expires,
one expected hold after it was made.
*/
type entryPlan struct {
	target    *decimal.Decimal
	submitted *decimal.Decimal
	expires   time.Time
	lastBook  time.Time
	binding   string
}

/*
position accumulates the venue's authoritative fills for one symbol.
*/
type position struct {
	bought   *decimal.Decimal
	cost     *decimal.Decimal
	sold     *decimal.Decimal
	proceeds *decimal.Decimal
	fees     *decimal.Decimal
	orders   map[string]*order // Client order ID -> submitted order.
	exiting  bool
	trigger  string           // Trigger of the exit in progress.
	buying   *decimal.Decimal // Submitted, unfilled buy quantity.
	inflight *decimal.Decimal // Submitted, unfilled sell quantity.
	plan     *entryPlan
	budget   float64
	source   string
	hold     time.Duration
	capacity Capacity
	sells    []Sell
	shadow   ShadowLedger
	// breachSince is the venue time exit capacity first fell short of the
	// open quantity by a material margin in the current run of versions;
	// zero while it does not, and reset by each monitor sell.
	breachSince time.Time
	life        *Lifecycle
	sizing      map[string]any
	firstFill   time.Time
}

func newPosition() *position {
	return &position{
		bought:   decimal.NewFromInt64(0),
		cost:     decimal.NewFromInt64(0),
		sold:     decimal.NewFromInt64(0),
		proceeds: decimal.NewFromInt64(0),
		fees:     decimal.NewFromInt64(0),
		buying:   decimal.NewFromInt64(0),
		inflight: decimal.NewFromInt64(0),
		orders:   make(map[string]*order),
	}
}

/*
open is the filled quantity not yet sold or being sold.
*/
func (held *position) open() *decimal.Decimal {
	return held.bought.Sub(held.sold).Sub(held.inflight)
}

/*
Desk owns open positions: it sizes entries from the book (exit capacity
within the slippage budget, flow noise and cash), works them in child market
orders, takes the learned exit, runs the capacity monitor, accumulates venue
fills beside their shadow fills, and reports each completed round trip.
*/
type Desk struct {
	*runtime.System
	transport        Transport
	price            *Price
	Balance          *Balance
	mu               sync.Mutex
	positions        map[string]*position
	positionsVersion atomic.Uint64
	closed           func(Closure)
	books            Depth
	shadow           *Shadow
	closures         []Closure
	lifecycles       []*Lifecycle
	pendingMu        sync.Mutex
	pending          map[string]struct{}
	wake             chan struct{}
	// shortBook marks symbols already reported as sized from a retained
	// book shorter than the expected hold, so it is said once per symbol.
	shortBook sync.Map
}

func NewDesk(
	ctx context.Context,
	transport Transport,
	price *Price,
	balance ...*Balance,
) *Desk {
	desk := &Desk{
		transport: transport,
		price:     price,
		positions: make(map[string]*position),
		shadow:    NewShadow(),
		pending:   make(map[string]struct{}),
		wake:      make(chan struct{}, 1),
	}

	if len(balance) > 0 {
		desk.Balance = balance[0]
	}

	desk.System = runtime.NewSystem(ctx, "desk", desk)
	desk.Transition(runtime.READY)
	return desk
}

/*
UseDepth connects the book history entries are sized from, the capacity
monitor reads and shadow fills walk, and starts working book updates
delivered through Wake. Without it Enter refuses every entry.
*/
func (desk *Desk) UseDepth(books Depth) {
	desk.mu.Lock()
	started := desk.books != nil
	desk.books = books
	desk.mu.Unlock()

	if !started && books != nil {
		go desk.run()
	}
}

/*
Wake tells the Desk that symbol's book has a new version. It never blocks:
the Book calls it from its update path.
*/
func (desk *Desk) Wake(symbol string) {
	desk.pendingMu.Lock()
	desk.pending[symbol] = struct{}{}
	desk.pendingMu.Unlock()

	select {
	case desk.wake <- struct{}{}:
	default:
	}
}

func (desk *Desk) run() {
	for {
		select {
		case <-desk.Context().Done():
			return
		case <-desk.wake:
		}

		desk.pendingMu.Lock()
		symbols := desk.pending
		desk.pending = make(map[string]struct{})
		desk.pendingMu.Unlock()

		for symbol := range symbols {
			desk.step(symbol)
		}
	}
}

/*
OnClose registers the consumer of realized round-trip outcomes.
*/
func (desk *Desk) OnClose(handler func(Closure)) {
	desk.closed = handler
}

/*
State reports the lifecycle phase of the symbol's position.
*/
func (desk *Desk) State(symbol string) PositionState {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	held, ok := desk.positions[symbol]

	if !ok {
		return FLAT
	}

	if held.exiting {
		return EXITING
	}

	if held.bought.Sign() <= 0 {
		return ENTERING
	}

	return HOLDING
}

/*
Enter sizes an entry from the book and the matched edge and starts working it
in child market orders. The size is the smallest of: the exit capacity within
the slippage budget at its weakest over the last expected hold, the standard
deviation of signed traded volume over one expected hold (participation
within flow noise), and what the available cash buys at the budget's entry
limit. Every refusal is an error naming its reason.
*/
func (desk *Desk) Enter(symbol string, edge Edge) error {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	if _, ok := desk.positions[symbol]; ok {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable, "[desk] position already exists for "+symbol, nil,
		))
	}

	held, err := desk.plan(symbol, edge)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Validation, "[desk] unable to size entry for "+symbol, err,
		))
	}

	held.life = newLifecycle(symbol)
	desk.lifecycles = append(desk.lifecycles, held.life)

	if edge.Match != nil {
		held.life.record(EventEntryMatch, "learned enter matched on "+edge.Match.Path, edge.Match.At, matchFields(*edge.Match))
	}

	bookAt, _ := held.sizing["book_at"].(time.Time)
	held.life.record(EventSizing, fmt.Sprintf(
		"sized %v bound by %v", held.sizing["quantity"], held.sizing["binding"],
	), bookAt, held.sizing)

	desk.positions[symbol] = held
	desk.positionsVersion.Add(1)
	desk.Wake(symbol)

	return nil
}

func (desk *Desk) plan(symbol string, edge Edge) (*position, error) {
	if desk.books == nil {
		return nil, errnie.Err(errnie.Validation, "book depth is required to size an entry", nil)
	}

	fee, ok := desk.price.FeeRate(symbol)

	if !ok {
		return nil, errnie.Err(errnie.NotFound, "fee unavailable", nil)
	}

	budget, source, err := SlippageBudget(fee, edge.Gains)

	if err != nil {
		return nil, err
	}

	if source == BudgetFeeFallback {
		errnie.Warn("[desk] " + symbol + ": no matched edge statistics, slippage budget is one taker fee")
	}

	hold, ok := ExpectedHold(edge.Holds)

	if !ok {
		return nil, errnie.Err(errnie.Validation, "no readable matched durations, no expected hold", nil)
	}

	var latest *BookView
	desk.books.Latest(symbol, func(view *BookView) { latest = view })

	if latest == nil {
		return nil, errnie.Err(errnie.NotFound, "no verified book version", nil)
	}

	now := latest.At
	exitCapacity, versions := math.Inf(1), 0

	covered := desk.books.BookWindow(symbol, now.Add(-hold), now, func(view *BookView) {
		versions++

		if capacity := ExitCapacity(desk.shadow.Adjusted(symbol, view), budget); capacity.Defined {
			exitCapacity = min(exitCapacity, capacity.Quantity)
			return
		}

		exitCapacity = 0
	})

	if versions == 0 {
		return nil, errnie.Err(errnie.NotFound, "no book versions over the expected hold", nil)
	}

	// Retention grows from the first lookup, so after startup the book
	// covers less than a hold until one hold has passed.
	if !covered {
		if _, said := desk.shortBook.LoadOrStore(symbol, struct{}{}); !said {
			errnie.Info(fmt.Sprintf("[desk] %s: exit capacity read over the retained book only, shorter than the expected hold %s", symbol, hold))
		}
	} else {
		desk.shortBook.Delete(symbol)
	}

	noise, _, ok := desk.price.Flow.Noise(symbol, now, hold)

	if !ok {
		return nil, errnie.Err(errnie.Validation, "participation undefined: no signed flow observed yet", nil)
	}

	entry := EntryCapacity(latest, budget)

	if !entry.Defined {
		return nil, errnie.Err(errnie.UnprocessableContent, "book has no defined touch", nil)
	}

	cash := desk.cash()

	if cash <= 0 {
		return nil, errnie.Err(errnie.Validation, "no cash available", nil)
	}

	unit := entry.Reference * (1 + budget)
	limits := []struct {
		name  string
		value float64
	}{
		{"exit_capacity", exitCapacity},
		{"participation", noise},
		{"cash", cash / (unit * (1 + fee))},
	}

	target, binding := math.Inf(1), ""

	for _, limit := range limits {
		if limit.value < target {
			target, binding = limit.value, limit.name
		}
	}

	quantity := desk.floor(symbol, target)

	if quantity == nil || !desk.tradable(symbol, quantity, unit) {
		return nil, errnie.Err(errnie.Validation, fmt.Sprintf(
			"sized quantity %g (bound by %s) is below the instrument minimum", target, binding,
		), nil)
	}

	errnie.Info(fmt.Sprintf(
		"[desk] %s: entry %s (bound by %s; exit_capacity=%g participation=%g cash=%g) budget=%g (%s) hold=%s",
		symbol, quantity.String(), binding, limits[0].value, limits[1].value, limits[2].value, budget, source, hold,
	))

	held := newPosition()
	held.budget, held.source, held.hold = budget, source, hold
	held.plan = &entryPlan{
		target:    quantity,
		submitted: decimal.NewFromInt64(0),
		expires:   now.Add(hold),
		binding:   binding,
	}
	held.sizing = map[string]any{
		"quantity":           quantity.String(),
		"binding":            binding,
		"exit_capacity":      limits[0].value,
		"exit_book_versions": versions,
		"exit_book_covered":  covered,
		"participation":      limits[1].value,
		"cash":               cash,
		"cash_quantity":      limits[2].value,
		"reference":          entry.Reference,
		"entry_limit_price":  unit,
		"fee":                fee,
		"budget":             budget,
		"budget_source":      source,
		"expected_hold_s":    hold.Seconds(),
		"matched_gains":      len(edge.Gains),
		"matched_holds":      len(edge.Holds),
		"book_at":            now,
	}
	return held, nil
}

/*
cash is the quote cash an entry may spend: the venue balance when known,
otherwise the reference cash.
*/
func (desk *Desk) cash() float64 {
	if desk.Balance != nil {
		if cash := desk.Balance.Cash(); cash != nil {
			return cash.Float64()
		}
	}

	if cash := desk.price.ReferenceCash(); cash != nil {
		return cash.Float64()
	}

	return 0
}

/*
floor normalizes quantity to the pair's lot size without rounding up.
*/
func (desk *Desk) floor(symbol string, quantity float64) *decimal.Decimal {
	if quantity <= 0 || math.IsInf(quantity, 0) || math.IsNaN(quantity) {
		return nil
	}

	exact := decimal.NewFromFloat64(quantity)
	normalized, err := desk.price.normalizer.FormatSize(symbol, exact)

	if err != nil {
		return nil
	}

	if normalized.Cmp(exact) > 0 {
		normalized = normalized.Sub(normalized.GetSmallestIncrement())
	}

	if normalized.Sign() <= 0 {
		return nil
	}

	return normalized
}

/*
tradable checks the instrument minimums when the instrument registry is
known; without one only a positive quantity is required.
*/
func (desk *Desk) tradable(symbol string, quantity *decimal.Decimal, unit float64) bool {
	if quantity == nil || quantity.Sign() <= 0 {
		return false
	}

	if desk.price.Instrument == nil {
		return true
	}

	return desk.price.Tradable(symbol, quantity, decimal.NewFromFloat64(unit))
}

/*
step works one book update for symbol: the capacity monitor for a filled
position, then the next child of an entry still being worked.
*/
func (desk *Desk) step(symbol string) {
	var latest *BookView
	desk.books.Latest(symbol, func(view *BookView) { latest = view })

	if latest == nil {
		return
	}

	desk.mu.Lock()

	held, ok := desk.positions[symbol]

	if !ok {
		desk.mu.Unlock()
		return
	}

	view := desk.shadow.Adjusted(symbol, latest)
	side, quantity, trigger := desk.monitor(symbol, held, view)

	if quantity == nil {
		side, quantity, trigger = desk.child(symbol, held, view)
	}

	desk.mu.Unlock()

	if quantity != nil {
		go desk.submit(symbol, side, quantity, trigger)
	}
}

/*
monitor is the secondary risk mechanism. It reads the exit capacity of the
newest version against the open quantity and only ever sells: it trims to
capacity, and when capacity is below the instrument minimum it exits fully.

A thin book flickers version to version, and each sell pays the taker fee and
shows the position, so the monitor acts only on a material, persistent
shortfall: open quantity above capacity by more than core.Tolerance of the
open quantity and by at least a tradable trim, continuously for
core.Tolerance of the expected hold. Each sell restarts the run, so two
monitor sells on a symbol are always at least that far apart. Any firing ends the entry plan. The caller holds the lock.
*/
func (desk *Desk) monitor(symbol string, held *position, view *BookView) (Direction, *decimal.Decimal, string) {
	if held.exiting || held.bought.Sign() <= 0 || held.inflight.Sign() > 0 {
		return "", nil, ""
	}

	capacity := ExitCapacity(view, held.budget)

	if capacity.Quantity != held.capacity.Quantity || capacity.Defined != held.capacity.Defined {
		desk.positionsVersion.Add(1)
	}

	held.capacity = capacity
	open := held.open()

	if !capacity.Defined || open.Sign() <= 0 || capacity.Quantity >= open.Float64() {
		held.breachSince = time.Time{}
		return "", nil, ""
	}

	keep := desk.floor(symbol, capacity.Quantity)
	full := keep == nil || !desk.tradable(symbol, keep, capacity.Reference)
	shortfall := open.Float64() - capacity.Quantity
	material := shortfall > core.Tolerance*open.Float64() &&
		(full || desk.tradable(symbol, open.Sub(keep), capacity.Reference))

	if !material {
		held.breachSince = time.Time{}
		return "", nil, ""
	}

	if held.breachSince.IsZero() {
		held.breachSince = view.At
	}

	persistence := time.Duration(core.Tolerance * float64(held.hold))

	if view.At.Sub(held.breachSince) < persistence {
		return "", nil, ""
	}
	sell := Sell{
		At:       view.At,
		Capacity: capacity,
		Ratio:    capacity.Quantity / open.Float64(),
		Budget:   held.budget,
	}

	if full {
		sell.Trigger, sell.Quantity = TriggerCapacityExit, open
		held.exiting, held.trigger = true, TriggerCapacityExit
		held.status("exiting")
	} else {
		sell.Trigger, sell.Quantity = TriggerCapacityTrim, open.Sub(keep)
	}

	held.life.record(EventRiskSell, fmt.Sprintf(
		"%s sells %s: exit capacity %g vs open %s held %s", sell.Trigger, sell.Quantity.String(),
		capacity.Quantity, open.String(), view.At.Sub(held.breachSince),
	), view.At, map[string]any{
		"trigger":      sell.Trigger,
		"quantity":     sell.Quantity.String(),
		"capacity":     capacity.Quantity,
		"reference":    capacity.Reference,
		"open":         open.String(),
		"ratio":        sell.Ratio,
		"budget":       held.budget,
		"breach_since": held.breachSince,
		"persistence":  persistence.Seconds(),
	})
	held.breachSince = time.Time{}
	held.plan = nil
	held.inflight = held.inflight.Add(sell.Quantity)
	held.sells = append(held.sells, sell)
	desk.positionsVersion.Add(1)

	errnie.Warn(fmt.Sprintf(
		"[desk] %s: %s sells %s (exit capacity %g vs open %s, ratio %.3f, budget %g)",
		symbol, sell.Trigger, sell.Quantity.String(), capacity.Quantity, open.String(), sell.Ratio, held.budget,
	))

	return SELL, sell.Quantity, sell.Trigger
}

/*
child sizes the next child buy of the entry plan. The caller holds the lock.
*/
func (desk *Desk) child(symbol string, held *position, view *BookView) (Direction, *decimal.Decimal, string) {
	plan := held.plan

	if plan == nil || held.exiting {
		return "", nil, ""
	}

	if view.At.After(plan.expires) {
		held.plan = nil
		desk.positionsVersion.Add(1)
		errnie.Info(fmt.Sprintf(
			"[desk] %s: entry plan expired after one expected hold with %s of %s submitted",
			symbol, plan.submitted.String(), plan.target.String(),
		))
		held.life.record(EventPlanEnded, "entry plan expired after one expected hold", view.At, map[string]any{
			"submitted": plan.submitted.String(), "target": plan.target.String(),
		})

		if held.bought.Sign() <= 0 && held.buying.Sign() <= 0 {
			held.life.end(EventAbandoned)
			delete(desk.positions, symbol)
		}

		return "", nil, ""
	}

	if held.buying.Sign() > 0 || (plan.submitted.Sign() > 0 && !view.At.After(plan.lastBook)) {
		return "", nil, ""
	}

	remaining := plan.target.Sub(plan.submitted)

	if remaining.Sign() <= 0 {
		held.plan = nil
		desk.positionsVersion.Add(1)
		return "", nil, ""
	}

	entry := EntryCapacity(view, held.budget)

	if !entry.Defined {
		return "", nil, ""
	}

	// Plans of other symbols spend the same cash: a child never asks for
	// more than the cash left now buys at the budget's entry limit.
	unit := entry.Reference * (1 + held.budget)
	fee, _ := desk.price.FeeRate(symbol)
	affordable := desk.cash() / (unit * (1 + fee))
	quantity := desk.floor(symbol, min(remaining.Float64(), entry.Quantity, affordable))

	if quantity == nil || !desk.tradable(symbol, quantity, unit) {
		return "", nil, ""
	}

	plan.submitted = plan.submitted.Add(quantity)
	plan.lastBook = view.At
	held.buying = held.buying.Add(quantity)
	desk.positionsVersion.Add(1)
	held.life.record(EventChildOrder, "child buy "+quantity.String()+" of the entry plan", view.At, map[string]any{
		"quantity":       quantity.String(),
		"entry_capacity": entry.Quantity,
		"affordable":     affordable,
		"reference":      entry.Reference,
		"limit_price":    unit,
		"submitted":      plan.submitted.String(),
		"target":         plan.target.String(),
	})
	return BUY, quantity, ""
}

/*
Exit takes the learned exit: it sells everything still open. It always wins:
it ends the entry plan, and when a capacity trim is in flight it sells the
rest. When the position is already exiting (the monitor's full exit came
first) it records that it also fired, so both are visible.
*/
func (desk *Desk) Exit(symbol string) error {
	return desk.ExitBy(symbol, TriggerLearnedExit)
}

/*
ExitBy sells everything still open under trigger.
*/
func (desk *Desk) ExitBy(symbol, trigger string) error {
	desk.mu.Lock()

	held, ok := desk.positions[symbol]

	if ok && held.exiting {
		held.sells = append(held.sells, Sell{
			At: time.Now().UTC(), Trigger: trigger, Capacity: held.capacity, Budget: held.budget,
			Note: "position already exiting under " + held.trigger,
		})
		held.life.record(EventExit, trigger+" also fired; already exiting under "+held.trigger, time.Time{}, map[string]any{
			"trigger": trigger, "exiting_under": held.trigger,
		})
		desk.positionsVersion.Add(1)
		desk.mu.Unlock()

		errnie.Info(fmt.Sprintf("[desk] %s: %s also fired; already exiting under %s", symbol, trigger, held.trigger))
		return nil
	}

	if ok && held.bought.Sign() <= 0 {
		held.plan = nil
		held.life.record(EventPlanEnded, trigger+" before any fill; remaining entry cancelled", time.Time{}, map[string]any{
			"trigger": trigger,
		})

		if held.buying.Sign() <= 0 {
			held.life.end(EventAbandoned)
			delete(desk.positions, symbol)
		}

		desk.positionsVersion.Add(1)
		desk.mu.Unlock()

		return errnie.Error(errnie.Err(
			errnie.NotAcceptable, "[desk] no filled position to exit for "+symbol+"; remaining entry cancelled", nil,
		))
	}

	if !ok {
		quantity := desk.recover(symbol, trigger)
		desk.mu.Unlock()

		if quantity == nil {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable, "[desk] no filled position to exit for "+symbol, nil,
			))
		}

		go desk.submit(symbol, SELL, quantity, trigger)
		return nil
	}

	held.exiting, held.trigger, held.plan = true, trigger, nil
	open := held.open()
	sell := Sell{At: time.Now().UTC(), Trigger: trigger, Capacity: held.capacity, Budget: held.budget}

	if held.capacity.Defined && held.bought.Sign() > 0 {
		sell.Ratio = held.capacity.Quantity / held.bought.Sub(held.sold).Float64()
	}

	exitFields := map[string]any{
		"trigger":  trigger,
		"open":     open.String(),
		"capacity": held.capacity.Quantity,
		"defined":  held.capacity.Defined,
		"ratio":    sell.Ratio,
		"budget":   held.budget,
	}

	if open.Sign() <= 0 {
		sell.Note = "nothing open beyond in-flight sells"
		held.life.record(EventExit, trigger+": "+sell.Note, time.Time{}, exitFields)
		held.sells = append(held.sells, sell)
		desk.positionsVersion.Add(1)
		desk.mu.Unlock()
		return nil
	}

	sell.Quantity = open
	held.life.record(EventExit, trigger+" sells "+open.String(), time.Time{}, exitFields)
	held.status("exiting")
	held.inflight = held.inflight.Add(open)
	held.sells = append(held.sells, sell)
	desk.positionsVersion.Add(1)
	desk.mu.Unlock()

	go desk.submit(symbol, SELL, open, trigger)
	return nil
}

/*
recover adopts a holding the venue balance shows but the Desk does not track
(a restart), as an exiting position for the full balance. The caller holds
the lock.
*/
func (desk *Desk) recover(symbol, trigger string) *decimal.Decimal {
	if desk.Balance == nil {
		return nil
	}

	snap := desk.Balance.Snapshot()

	if snap == nil || snap.Assets == nil {
		return nil
	}

	asset := symbol

	if strings.Contains(symbol, "/") {
		asset = strings.Split(symbol, "/")[0]
	}

	qty, exists := snap.Assets[asset]

	if !exists || qty == nil || qty.Sign() <= 0 {
		return nil
	}

	held := newPosition()
	held.bought, held.exiting, held.trigger = qty, true, trigger
	held.inflight = qty
	held.sells = append(held.sells, Sell{
		At: time.Now().UTC(), Trigger: trigger, Quantity: qty, Note: "adopted from venue balance",
	})
	held.life = newLifecycle(symbol)
	held.life.Status = "exiting"
	held.life.record(EventExit, trigger+" on a holding adopted from the venue balance", time.Time{}, map[string]any{
		"trigger": trigger, "quantity": qty.String(),
	})
	desk.lifecycles = append(desk.lifecycles, held.life)
	desk.positions[symbol] = held
	desk.positionsVersion.Add(1)

	return qty
}

/*
Unrealized marks the open position against current bid depth, net of fees.
*/
func (desk *Desk) Unrealized(symbol string) (*decimal.Decimal, *decimal.Decimal, error) {
	desk.mu.Lock()

	held, ok := desk.positions[symbol]

	if !ok || held.bought.Sign() <= 0 {
		desk.mu.Unlock()

		return nil, nil, errnie.Error(errnie.Err(
			errnie.NotFound, "[desk] no filled position for "+symbol, nil,
		))
	}

	quantity := held.bought
	basis := held.cost.Add(held.fees)

	desk.mu.Unlock()

	net, _, err := desk.price.Liquidate(symbol, quantity)

	if err != nil {
		return nil, nil, errnie.Error(err)
	}

	return net.Sub(basis), basis, nil
}

/*
Apply accumulates execution fills into their positions and closes a position
once an exit has sold everything that was bought. Fills are attributed by the
client order ID the Desk submitted, never by inferring the side. Every fill is
also walked through the as-of book into the shadow ledger.
*/
func (desk *Desk) Apply(execution *kraken.Execution) {
	if execution == nil {
		return
	}

	for _, row := range execution.Data {
		if row.LastQty == nil || row.LastQty.Sign() <= 0 {
			continue
		}

		if row.Cost == nil || row.Cost.Sign() <= 0 {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent, "[desk] fill without cost for "+row.Symbol, nil,
			))

			continue
		}

		closure, done, followUp := desk.fill(row)

		if followUp != nil {
			go desk.submit(row.Symbol, SELL, followUp.quantity, followUp.trigger)
		}

		if done && desk.closed != nil {
			desk.closed(closure)
		}
	}
}

/*
followUp is a sell a fill makes necessary.
*/
type followUp struct {
	quantity *decimal.Decimal
	trigger  string
}

/*
fill applies one venue fill. A buy filled after an exit started is sold under
that exit's trigger (followUp).
*/
func (desk *Desk) fill(row kraken.ExecutionData) (Closure, bool, *followUp) {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	held, ok := desk.positions[row.Symbol]

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound, "[desk] fill for unknown position "+row.Symbol, nil,
		))

		return Closure{}, false, nil
	}

	submitted, ok := held.orders[row.ClientOrderID]

	if !ok {
		errnie.Error(errnie.Err(
			errnie.NotFound, "[desk] fill for unknown order "+row.ClientOrderID+" on "+row.Symbol, nil,
		))

		return Closure{}, false, nil
	}

	if row.FeeUsdEquiv != nil {
		held.fees = held.fees.Add(row.FeeUsdEquiv)
	}

	shadow, priced := desk.shadowFill(row, held, submitted.side)
	desk.positionsVersion.Add(1)

	if held.firstFill.IsZero() {
		held.firstFill = time.Now().UTC()

		if !row.Timestamp.IsZero() {
			held.firstFill = row.Timestamp.UTC()
		}

		if !held.exiting {
			held.status("holding")
		}
	}

	fillFields := map[string]any{
		"side":            string(submitted.side),
		"trigger":         submitted.trigger,
		"client_order_id": row.ClientOrderID,
		"quantity":        row.LastQty.String(),
		"venue_cost":      row.Cost.String(),
		"venue_price":     row.Cost.Float64() / row.LastQty.Float64(),
		"shadow_priced":   priced,
	}

	if row.FeeUsdEquiv != nil {
		fillFields["venue_fee"] = row.FeeUsdEquiv.String()
	}

	if priced {
		fillFields["shadow_gross"] = shadow.Gross
		fillFields["shadow_quantity"] = shadow.Quantity
		fillFields["shadow_short"] = shadow.Short

		if shadow.Quantity > 0 {
			fillFields["shadow_price"] = shadow.Gross / shadow.Quantity
		}
	}

	held.life.record(EventFill, fmt.Sprintf("%s filled %s", submitted.side, row.LastQty.String()), row.Timestamp, fillFields)
	if submitted.side == BUY {
		held.bought = held.bought.Add(row.LastQty)
		held.cost = held.cost.Add(row.Cost)
		held.buying = nonNegative(held.buying.Sub(row.LastQty))

		if held.exiting {
			held.inflight = held.inflight.Add(row.LastQty)
			held.sells = append(held.sells, Sell{
				At: time.Now().UTC(), Trigger: held.trigger, Quantity: row.LastQty,
				Note: "entry child filled after the exit started",
			})

			return Closure{}, false, &followUp{quantity: row.LastQty, trigger: held.trigger}
		}

		return Closure{}, false, nil
	}

	held.sold = held.sold.Add(row.LastQty)
	held.proceeds = held.proceeds.Add(row.Cost)
	held.inflight = nonNegative(held.inflight.Sub(row.LastQty))

	if !held.exiting || held.sold.Cmp(held.bought) < 0 || held.buying.Sign() > 0 {
		return Closure{}, false, nil
	}

	delete(desk.positions, row.Symbol)

	closure := Closure{
		Symbol:   row.Symbol,
		Cost:     held.cost,
		Proceeds: held.proceeds,
		Fees:     held.fees,
		Realized: held.proceeds.Sub(held.cost).Sub(held.fees),
		Sells:    held.sells,
		Shadow:   held.shadow,
		ClosedAt: time.Now().UTC(),
	}

	desk.closures = append(desk.closures, closure)
	desk.conclude(held, closure)
	return closure, true, nil
}

/*
shadowFill walks one venue fill through the book version in effect at the
venue's fill time (the newest version when the fill carries none). The caller
holds the lock.
*/
func (desk *Desk) shadowFill(row kraken.ExecutionData, held *position, side Direction) (ShadowFill, bool) {
	var view *BookView

	if desk.books != nil {
		read := func(found *BookView) { view = found }

		if row.Timestamp.IsZero() {
			desk.books.Latest(row.Symbol, read)
		} else {
			desk.books.BookAt(row.Symbol, row.Timestamp, read)
		}
	}

	if view == nil {
		held.shadow.Unpriced++
		return ShadowFill{}, false
	}

	fee, _ := desk.price.FeeRate(row.Symbol)
	fill := desk.shadow.Fill(row.Symbol, side, view, row.LastQty.Float64())
	held.shadow.Fees += fill.Gross * fee
	held.shadow.Short += fill.Short

	if side == BUY {
		held.shadow.Cost += fill.Gross
		held.shadow.Bought += fill.Quantity
		return fill, true
	}

	held.shadow.Proceeds += fill.Gross
	held.shadow.Sold += fill.Quantity
	return fill, true
}

/*
status moves the lifecycle's status. The caller holds the lock.
*/
func (held *position) status(status string) {
	if held.life != nil && held.life.ClosedAt == nil {
		held.life.Status = status
	}
}

/*
conclude records the closed position's outcome. The caller holds the lock.
*/
func (desk *Desk) conclude(held *position, closure Closure) {
	if held.life == nil {
		return
	}

	outcome := &Outcome{
		VenueCost:      closure.Cost.Float64(),
		VenueProceeds:  closure.Proceeds.Float64(),
		VenueFees:      closure.Fees.Float64(),
		VenueRealized:  closure.Realized.Float64(),
		ShadowDefined:  closure.Shadow.Defined(),
		ShadowCost:     closure.Shadow.Cost,
		ShadowProceeds: closure.Shadow.Proceeds,
		ShadowFees:     closure.Shadow.Fees,
		ShadowShort:    closure.Shadow.Short,
		ShadowUnpriced: closure.Shadow.Unpriced,
		ExpectedHold:   held.hold,
		Triggers:       make([]string, 0, len(closure.Sells)),
	}

	if outcome.ShadowDefined {
		outcome.ShadowRealized = closure.Shadow.Realized()
	}

	if !held.firstFill.IsZero() {
		outcome.Hold = closure.ClosedAt.Sub(held.firstFill)
	}

	for _, sell := range closure.Sells {
		outcome.Triggers = append(outcome.Triggers, sell.Trigger)
	}

	held.life.Outcome = outcome
	held.life.record(EventClosed, fmt.Sprintf(
		"closed: venue %.4f, shadow %.4f, held %s of expected %s",
		outcome.VenueRealized, outcome.ShadowRealized, outcome.Hold.Round(time.Second), held.hold.Round(time.Second),
	), time.Time{}, nil)
	held.life.end("closed")
}

func nonNegative(value *decimal.Decimal) *decimal.Decimal {
	if value.Sign() < 0 {
		return decimal.NewFromInt64(0)
	}

	return value
}

/*
submit writes one market order envelope to the transport. A failed buy
returns its quantity to the entry plan and releases a position that never
filled; a failed sell returns its quantity to the open position and ends the
exit it belonged to.
*/
func (desk *Desk) submit(symbol string, side Direction, quantity *decimal.Decimal, trigger string) {
	clientOrderID := uuid.NewString()

	desk.mu.Lock()

	if held, ok := desk.positions[symbol]; ok {
		held.orders[clientOrderID] = &order{side: side, quantity: quantity, trigger: trigger}
		held.life.record(EventOrder, fmt.Sprintf("market %s %s", side, quantity.String()), time.Time{}, map[string]any{
			"side": string(side), "quantity": quantity.String(), "trigger": trigger, "client_order_id": clientOrderID,
		})
	}

	desk.mu.Unlock()

	message := kraken.NewAddOrderMessage("", &kraken.AddOrderRequest{
		Pair:    symbol,
		Type:    string(side),
		OrdType: "market",
		Volume:  quantity.String(),
		ClOrdId: clientOrderID,
	})

	payload, err := sonic.Marshal(message)

	if err == nil {
		err = desk.transport.Write(payload)
	}

	if err == nil {
		return
	}

	errnie.Error(errnie.Err(
		errnie.IO, "[desk] order submission failed for "+symbol, err,
	))

	desk.mu.Lock()
	defer desk.mu.Unlock()

	held, ok := desk.positions[symbol]

	if !ok {
		return
	}

	delete(held.orders, clientOrderID)
	desk.positionsVersion.Add(1)
	held.life.record(EventOrderFailed, "order submission failed: "+err.Error(), time.Time{}, map[string]any{
		"side": string(side), "quantity": quantity.String(), "trigger": trigger, "client_order_id": clientOrderID,
	})
	if side == SELL {
		held.inflight = nonNegative(held.inflight.Sub(quantity))

		if held.exiting && held.trigger == trigger {
			held.exiting, held.trigger = false, ""
			held.status("holding")
		}

		return
	}

	held.buying = nonNegative(held.buying.Sub(quantity))

	if held.plan != nil {
		held.plan.submitted = nonNegative(held.plan.submitted.Sub(quantity))
	}

	if held.bought.Sign() <= 0 && held.buying.Sign() <= 0 && held.plan == nil {
		held.life.end(EventAbandoned)
		delete(desk.positions, symbol)
	}
}

/*
PositionsVersion reports the monotonic revision of the active positions.
*/
func (desk *Desk) PositionsVersion() uint64 {
	if desk == nil {
		return 0
	}

	return desk.positionsVersion.Load()
}

/*
marks prices the open quantity two ways for the wire: as the paper venue
fills a market sell (best bid for any size, net of the exit fee) and as the
shadow walks it through the newest book after earlier shadow fills. The
caller holds the lock.
*/
func (desk *Desk) marks(symbol string, held *position) (venue, shadow float64, ok bool) {
	if desk.books == nil {
		return 0, 0, false
	}

	var latest *BookView
	desk.books.Latest(symbol, func(view *BookView) { latest = view })

	if latest == nil || len(latest.Bids) == 0 {
		return 0, 0, false
	}

	fee, _ := desk.price.FeeRate(symbol)
	venue = latest.Bids[0].Price * held.bought.Sub(held.sold).Float64() * (1 - fee)
	quote := desk.shadow.Quote(symbol, SELL, latest, held.shadow.Bought-held.shadow.Sold)

	return venue, quote.Gross * (1 - fee), true
}

func sellEvents(sells []Sell) []*wire.SellEventT {
	events := make([]*wire.SellEventT, 0, len(sells))

	for _, sell := range sells {
		event := &wire.SellEventT{
			At:            sell.At.UnixNano(),
			Trigger:       sell.Trigger,
			CapacityQty:   sell.Capacity.Quantity,
			CapacityRatio: sell.Ratio,
			Budget:        sell.Budget,
			Note:          sell.Note,
		}

		if sell.Quantity != nil {
			event.Qty = sell.Quantity.String()
		}

		events = append(events, event)
	}

	return events
}

/*
holdingWire fills the sizing, capacity, sell and P&L fields every tracked
position carries. The caller holds the lock.
*/
func (desk *Desk) holdingWire(symbol string, held *position, holding *wire.HoldingT) {
	holding.CapacityQty = held.capacity.Quantity
	holding.CapacityDefined = held.capacity.Defined
	holding.CapacityBounded = held.capacity.Bounded

	if !held.capacity.At.IsZero() {
		holding.CapacityAt = held.capacity.At.UnixNano()
	}

	if open := held.bought.Sub(held.sold); held.capacity.Defined && open.Sign() > 0 {
		holding.CapacityRatio = held.capacity.Quantity / open.Float64()
	}

	holding.SlippageBudget = held.budget
	holding.BudgetSource = held.source
	holding.HoldSeconds = held.hold.Seconds()
	holding.Sells = sellEvents(held.sells)
	holding.ShadowDefined = held.shadow.Defined()

	venueMark, shadowMark, ok := desk.marks(symbol, held)

	if !ok {
		holding.ShadowDefined = false
		return
	}

	venue := held.proceeds.Float64() + venueMark - held.cost.Float64() - held.fees.Float64()
	holding.VenuePnl = fmt.Sprintf("%.8f", venue)

	if holding.ShadowDefined {
		holding.ShadowPnl = fmt.Sprintf("%.8f", held.shadow.Proceeds+shadowMark-held.shadow.Cost-held.shadow.Fees)
	}
}

func closureWire(closure Closure) *wire.HoldingT {
	holding := &wire.HoldingT{
		Status:        "closed",
		Symbol:        closure.Symbol,
		Asset:         closure.Symbol,
		Pnl:           closure.Realized.String(),
		VenuePnl:      closure.Realized.String(),
		ShadowDefined: closure.Shadow.Defined(),
		Sells:         sellEvents(closure.Sells),
		ClosedAt:      closure.ClosedAt.UnixNano(),
	}

	if holding.ShadowDefined {
		holding.ShadowPnl = fmt.Sprintf("%.8f", closure.Shadow.Realized())
	}

	return holding
}

/*
PositionsWire returns the active open positions and spot holdings formatted
for the telemetry websocket feed.
*/
func (desk *Desk) PositionsWire() *wire.PositionsFrameT {
	if desk == nil {
		return &wire.PositionsFrameT{Rows: []*wire.PositionT{}, Closed: []*wire.HoldingT{}}
	}

	desk.mu.Lock()
	defer desk.mu.Unlock()

	rows := make([]*wire.PositionT, 0)
	handled := make(map[string]bool)

	for symbol, held := range desk.positions {
		if held.bought == nil || held.bought.Sign() <= 0 {
			continue
		}

		handled[symbol] = true
		status := "holding"

		if held.exiting {
			status = "exiting"
		}

		qtyStr := held.bought.Sub(held.sold).String()
		basis := held.cost.Add(held.fees)
		entryPriceStr := ""

		if held.bought.Sign() > 0 {
			entryPriceStr = basis.Div(held.bought).String()
		}

		markStr := entryPriceStr
		pnlStr := "0.00"
		returnPct := 0.0

		if desk.price != nil {
			mark := desk.price.CurrentMark(symbol)

			if mark != nil {
				markStr = mark.String()
			}

			net, _, err := desk.price.Liquidate(symbol, held.bought)

			if err == nil {
				pnl := net.Sub(basis)
				pnlStr = pnl.String()

				if basis.Sign() > 0 {
					returnPct = pnl.SetScale(decimal.DefaultScale).Div(basis).Float64() * 100
				}
			} else if mark != nil {
				proceeds := mark.Mul(held.bought)
				pnl := proceeds.Sub(basis)
				pnlStr = pnl.String()

				if basis.Sign() > 0 {
					returnPct = pnl.SetScale(decimal.DefaultScale).Div(basis).Float64() * 100
				}
			}
		}

		holding := &wire.HoldingT{
			Status:      status,
			Symbol:      symbol,
			Asset:       symbol,
			Qty:         qtyStr,
			SellableQty: qtyStr,
			EntryPrice:  entryPriceStr,
			Mark:        markStr,
			Pnl:         pnlStr,
			ReturnPct:   returnPct,
		}

		desk.holdingWire(symbol, held, holding)
		rows = append(rows, &wire.PositionT{Status: status, Holding: holding})
	}

	if desk.Balance != nil {
		snap := desk.Balance.Snapshot()

		if snap != nil && snap.Wallet != nil {
			for _, item := range snap.Wallet.Data {
				if item.Asset == desk.Balance.Quote || item.Balance == nil || item.Balance.Sign() <= 0 {
					continue
				}

				symbol := item.Asset + "/" + desk.Balance.Quote

				if handled[symbol] || handled[item.Asset] {
					continue
				}

				handled[symbol] = true
				qtyStr := item.Balance.String()
				sellableStr := qtyStr

				if item.Available != nil {
					sellableStr = item.Available.String()
				}

				markStr := ""
				pnlStr := "0.00"
				returnPct := 0.0

				if desk.price != nil {
					mark := desk.price.CurrentMark(symbol)

					if mark != nil {
						markStr = mark.String()
					}
				}

				rows = append(rows, &wire.PositionT{
					Status: "holding",
					Holding: &wire.HoldingT{
						Status:      "holding",
						Symbol:      symbol,
						Asset:       item.Asset,
						Qty:         qtyStr,
						SellableQty: sellableStr,
						EntryPrice:  markStr,
						Mark:        markStr,
						Pnl:         pnlStr,
						ReturnPct:   returnPct,
					},
				})
			}
		}
	}

	closed := make([]*wire.HoldingT, 0, len(desk.closures))

	for _, closure := range desk.closures {
		closed = append(closed, closureWire(closure))
	}

	return &wire.PositionsFrameT{Rows: rows, Closed: closed}
}
