package broker

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
Desk is the broker Actor entrypoint and persistent position owner. It drains
market tickers and account executions from their Workloads and
routes each message to the positions it owns so closed lots leave no abandoned
fan-out behind.
*/
type Desk struct {
	*PositionStore
	ctx            context.Context
	cancel         context.CancelFunc
	err            error
	status         types.Status
	api            *websocket.API
	instrument     *Instrument
	price          *Price
	balance        *Balance
	equity         atomic.Pointer[types.EquityReading]
	equityVersion  atomic.Uint64
	accountMu      sync.Mutex
	recovery       *Recovery
	positions      *sync.Map
	balanceRefresh atomic.Bool
	// lifecycleRecorders are the optional Hindsight trading-lifecycle sinks and
	// execution feedback observers. They record entry/fill/position-open/exit/close
	// transitions observationally; empty when neither Hindsight nor execution
	// realization is wired, and their failure never affects trading progress.
	lifecycleRecorders []LifecycleRecorder
	ObserveModule      func(string, time.Duration)
}

/*
LifecycleRecorder receives one real trading-lifecycle transition keyed by the
decision ID that caused it. Implementors own persistence; the desk reports only
after the corresponding transition and never branches trading state on it.
*/
type LifecycleRecorder interface {
	RecordLifecycle(event hindsight.LifecycleEvent)
}

/*
NewDesk constructs the serial broker owner from its explicit dependencies. The
desk owns no transport or account objects itself: the caller supplies the API,
instrument, price, balance, and recovery handler it will
route through, so the live wiring and tests share one construction path rather
than a half-built struct. positions is the shared open-position map the recovery
publisher repopulates on boot. store persists open-lot entry facts; the desk
embeds it so Save resolves against a live database rather than a nil pointer.
*/
func NewDesk(
	ctx context.Context,
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	recovery *Recovery,
	store *PositionStore,
	positions *sync.Map,
) (*Desk, error) {
	ctx, cancel := context.WithCancel(ctx)

	desk := &Desk{
		ctx:           ctx,
		cancel:        cancel,
		status:        types.READY,
		api:           api,
		instrument:    instrument,
		price:         price,
		balance:       balance,
		recovery:      recovery,
		PositionStore: store,
		positions:     positions,
	}

	if desk.positions == nil {
		desk.positions = &sync.Map{}
	}

	if recovery != nil {
		if err := recovery.Recover(); err != nil {
			desk.status = types.ERROR
			desk.err = errnie.Error(errnie.Err(
				errnie.Internal,
				"desk: failed to recover account positions",
				err,
			))

			return desk, desk.err
		}
	}

	if balance != nil {
		go func() {
			ticker := time.NewTicker(3 * time.Second)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					_ = desk.PublishEquity()
				}
			}
		}()
	}

	return desk, nil
}

func (desk *Desk) Name() string { return "desk" }

func (desk *Desk) Error() error { return desk.err }

func (desk *Desk) Cash() *decimal.Decimal {
	if desk == nil || desk.balance == nil {
		return nil
	}

	return desk.balance.Cash()
}

// StepTicker advances one ticker observation: the price cache and any open
// position's live mark.
func (desk *Desk) StepTicker(ticker kraken.TickerData) error {
	desk.price.Update(&ticker)
	found, ok := desk.positions.Load(ticker.Symbol)

	if !ok || found == nil {
		return nil
	}

	position, ok := found.(*Position)

	if !ok || position == nil {
		return nil
	}

	if err := position.publishGuardian(ticker); err != nil {
		// Priority ring saturation is a critical failure: the caller must
		// receive a non-nil error and observe the risk failure rather than a
		// mark silently disappearing behind a log line.
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"desk: position guardian priority ring saturated for "+ticker.Symbol,
			err,
		))
	}

	return nil
}

// StepExecution advances one execution against the symbol's open position.
func (desk *Desk) StepExecution(execution kraken.ExecutionData) error {
	found, ok := desk.positions.Load(execution.Symbol)

	if !ok || found == nil {
		return nil
	}

	position, ok := found.(*Position)

	if !ok || position == nil {
		return nil
	}

	if err := position.publishGuardian(execution); err != nil {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"desk: position guardian priority ring saturated for "+execution.Symbol,
			err,
		))
	}

	return nil
}

// StepLevel3 advances the symbol's continuously-resident execution reducer
// from the authoritative L3 stream and routes the same frame to the symbol's
// open position, which derives the committed post-frame executable state for
// the guardian. The reducer is advanced for every subscribed symbol regardless
// of whether a Position exists, exactly once in causal order; the signal
// pipeline consumes the raw frame directly elsewhere and never passes through
// this reducer.
func (desk *Desk) StepLevel3(level3 kraken.Level3Data) error {
	return desk.stepLevel3(level3, 0)
}

/*
StepLevel3Epoch advances one L3 frame together with its Hindsight StreamEpoch,
so a reconnect invalidates the previous epoch's execution state until the new
epoch's genuine snapshot seeds it.
*/
func (desk *Desk) StepLevel3Epoch(level3 kraken.Level3Data, epoch uint64) error {
	return desk.stepLevel3(level3, epoch)
}

func (desk *Desk) stepLevel3(level3 kraken.Level3Data, epoch uint64) error {
	if desk == nil || level3.Symbol == "" {
		return nil
	}

	if desk.api != nil && desk.api.Private() != nil {
		if applier, ok := desk.api.Private().(interface{ ApplyLevel3(kraken.Level3Data) }); ok {
			applier.ApplyLevel3(level3)
		}
	}

	found, ok := desk.positions.Load(level3.Symbol)

	if !ok || found == nil {
		return nil
	}

	position, ok := found.(*Position)

	if !ok || position == nil {
		return nil
	}

	if err := position.publishGuardian(level3); err != nil {
		return errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"desk: position guardian priority ring saturated for "+level3.Symbol,
			err,
		))
	}

	return nil
}

/*
Price exposes the desk's price surface so the market data path can keep it
current. Every executable calculation the broker makes reads from it, so it
has to see the same ticks the thesis does.
*/
func (desk *Desk) Price() *Price {
	return desk.price
}

/*
Balance exposes the desk's balance so callers outside the broker package can
read the current wallet state without accessing the unexported field.
*/
func (desk *Desk) Balance() *Balance {
	return desk.balance
}

/*
Instrument exposes the desk's instrument registry so callers outside the broker
package can reach pair metadata without accessing the unexported field.
*/
func (desk *Desk) Instrument() *Instrument {
	return desk.instrument
}

/*
Recovery exposes the desk's recovery handler.
*/
func (desk *Desk) Recovery() *Recovery {
	return desk.recovery
}

/*
Reduce sells part of an open lot without closing it, for a policy that sizes
its own exposure. A symbol with no open position has nothing to reduce and
says so rather than doing something else with the account. The guardian
serializes submission with fills; a nil return acknowledges queue admission.
*/
func (desk *Desk) Reduce(symbol string, volume *decimal.Decimal, correlationID ...string) error {
	if desk == nil || desk.positions == nil || symbol == "" {
		return errnie.Err(
			errnie.Validation,
			"desk: symbol required to reduce a position",
			nil,
		)
	}

	value, found := desk.positions.Load(symbol)

	if !found || value == nil {
		return errnie.Err(
			errnie.NotFound,
			"desk: no open position exists for "+symbol,
			nil,
		)
	}

	position, valid := value.(*Position)

	if !valid || position == nil || position.status() == types.CLOSED {
		return errnie.Err(
			errnie.NotFound,
			"desk: no open position exists for "+symbol,
			nil,
		)
	}

	decision := types.Decision{Action: types.ActionScale, ProposedQuantity: volume, Reduce: true}

	if len(correlationID) > 0 {
		decision.ID = correlationID[0]
	}

	return position.publishGuardian(decision)
}

/*
ManualExit executes an operator override for one open symbol. The desk's
execution lock serializes it against entry admission and repeated override
clicks, while Position owns the regulator transition and market order.
*/
func (desk *Desk) ManualExit(symbol string, correlationID ...string) error {
	if desk == nil || desk.positions == nil || symbol == "" {
		return errnie.Err(
			errnie.Validation,
			"desk: symbol required to close position",
			nil,
		)
	}

	value, found := desk.positions.Load(symbol)

	if !found || value == nil {
		return errnie.Err(
			errnie.NotFound,
			"desk: no open position exists for "+symbol,
			nil,
		)
	}

	position, valid := value.(*Position)

	if !valid || position == nil || position.status() == types.CLOSED {
		return errnie.Err(
			errnie.NotFound,
			"desk: no open position exists for "+symbol,
			nil,
		)
	}

	return position.ManualExit(correlationID...)
}

func (desk *Desk) OpenPositions() int {
	count := 0

	for position := range desk.Positions() {
		if position.status() != types.CLOSED {
			count++
		}
	}

	return count
}

/*
Holding reports how many lots the desk still carries for one symbol.

OpenPositions counts slots, which includes a lot whose order is still working.
This counts inventory, so it answers the only question an observer outside the
desk can ask about a symbol without guessing: is anything still on the book for
it, or has the position been run all the way out.
*/
func (desk *Desk) Holding(symbol string) int {
	if desk == nil || desk.positions == nil || symbol == "" {
		return 0
	}

	held := 0

	for position := range desk.Positions() {
		if position.pair.Symbol != symbol || position.status() == types.CLOSED {
			continue
		}

		held++
	}

	return held
}

/*
PositionQuantity reports the net base units held in open positions for symbol.
*/
func (desk *Desk) PositionQuantity(symbol string) float64 {
	if desk == nil || desk.positions == nil || symbol == "" {
		return 0
	}

	total := 0.0

	for position := range desk.Positions() {
		if position.pair.Symbol != symbol || position.status() == types.CLOSED {
			continue
		}

		if position.Holding != nil && position.Holding.Qty != nil {
			total += position.Holding.Qty.Float64()
		}
	}

	return total
}

/*
PublishEquity reports what the desk is worth if every open lot were closed now.

Cash alone understates the account while positions are open. Unrealized is the
profit/loss only; equity is cash plus the basis committed to open positions plus
that profit/loss.
*/
func (desk *Desk) PublishEquity() error {
	if desk == nil || desk.api == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"desk: API required",
			nil,
		))
	}

	desk.accountMu.Lock()
	defer desk.accountMu.Unlock()

	if desk.balance != nil {
		desk.balance.Update()
	}
	tradeBalance, err := desk.api.TradeBalance()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"desk: could not fetch trade balance",
			err,
		))
	}

	reading := types.NewEquityReading(tradeBalance)

	if reading == nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"desk: broker valuation did not include equity",
			nil,
		))
	}

	reading.At, reading.Version = time.Now().UTC(), desk.equityVersion.Add(1)
	reading.Positions = make(map[string]string)

	if desk.balance != nil {
		snapshot := desk.balance.snapshot.Load()
		reading.From = snapshot.from
		reading.Complete = reading.Complete && snapshot.status == types.READY
		// A complete total-balance map with no quote holding has zero quote cash.
		reading.Cash = "0"

		if cash := snapshot.assets[desk.balance.quote]; cash != nil {
			reading.Cash = cash.String()
		}
		for asset, quantity := range snapshot.assets {
			if asset == desk.balance.quote || quantity.Sign() <= 0 {
				continue
			}
			symbol := asset + "/" + desk.balance.quote

			if pair := desk.instrument.Pair(symbol); pair.Symbol == symbol {
				reading.Positions[symbol] = quantity.String()
			}
		}
	}
	desk.equity.Store(reading)

	return nil
}

/*
Equity returns the most recent complete valuation published by the broker.
*/
func (desk *Desk) Equity() *types.EquityReading {
	if desk == nil {
		return nil
	}

	return desk.equity.Load()
}

func (desk *Desk) Positions() iter.Seq[*Position] {
	return func(yield func(*Position) bool) {
		desk.positions.Range(func(key, value any) bool {
			position, ok := value.(*Position)

			if !ok || position == nil {
				return true
			}

			return yield(position)
		})
	}
}

/*
OpenPositionWire projects every open lot into its wire Position shape, excluding
closed lots. It is the account-state snapshot the positions panel consumes, one
wire.Position per open lot, in no guaranteed order.
*/
func (desk *Desk) OpenPositionWire() []*wire.PositionT {
	if desk == nil {
		return nil
	}

	positions := make([]*wire.PositionT, 0)

	for position := range desk.Positions() {
		if position.status() == types.CLOSED {
			continue
		}

		positions = append(positions, position.Wire())
	}

	return positions
}

/*
Execute turns an arbitrated decision round into desk-owned order work. Entries
are recorded before submission so the next arbitration round sees committed
capacity even while the venue acknowledgement or fill is still pending.
*/
func (desk *Desk) Execute(decision types.Decision) (err error) {
	started := time.Now()

	defer func() {
		if desk.ObserveModule != nil {
			desk.ObserveModule("desk", time.Since(started))
		}
	}()

	switch decision.Action {
	case types.ActionScale:
		value, found := desk.positions.Load(decision.Symbol)

		if !found {
			return &types.ExecutionRefusal{State: "no longer executable", Detail: "no lot exists to increase"}
		}
		position := value.(*Position)
		return position.publishGuardian(decision)
	case types.ActionEnter:
		// The issuing agent owns the exit: it re-evaluates every open position
		// on each book update and commands its own exit. The desk holds the
		// lot and executes what it is told; it does not carry protective
		// geometry of its own.
		if decision.ProposedQuantity == nil || decision.ProposedQuantity.Sign() <= 0 ||
			desk.price == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"desk: quantity and a live price are required for entry",
				nil,
			))
		}

		found, loaded := desk.positions.Load(decision.Symbol)

		if loaded {
			position, valid := found.(*Position)

			if valid && position != nil && position.status() != types.CLOSED {
				return &types.ExecutionRefusal{State: "account changed", Detail: "an active lot already exists for entry"}
			}
		}

		if decision.AllocationClass == "none" {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: decision was not allocated capital",
				nil,
			))
		}

		decision.AllocationClass = "capital"

		pair := desk.instrument.Pair(decision.Symbol)

		if pair.Symbol == "" || pair.TickSize.Sign() <= 0 {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: instrument and positive tick size required for entry",
				nil,
			))
		}

		fee := desk.price.Fee(decision.Symbol)

		if fee == nil || fee.Fee == nil || fee.Fee.Sign() < 0 ||
			fee.Fee.Cmp(decimal.NewFromInt64(100)) >= 0 {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: valid taker fee required for entry",
				nil,
			))
		}

		cost, costErr := desk.price.EntryCost(
			decision.Symbol,
			decision.ProposedQuantity,
		)

		if costErr != nil {
			if decision.Admit != nil {
				return &types.ExecutionRefusal{State: "no longer executable", Detail: costErr.Error()}
			}
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: current entry cannot execute",
				costErr,
			))
		}

		if decision.Admit != nil {
			if err := decision.Admit(cost); err != nil {
				return err
			}
		}
		decision.EntryCost = cost
		decision.ReferencePrice = decimal.NewFromInt64(0).Add(cost.BestAsk)
		decision.EntryPrice = decimal.NewFromInt64(0).Add(cost.EntryPrice)
		decision.Mark = decimal.NewFromInt64(0).Add(cost.BestBid)
		decision.ProposedNotional = decimal.NewFromInt64(0).Add(cost.GrossNotional).Add(
			cost.EntryFee,
		)
		position := NewPosition(
			desk.ctx,
			desk.api,
			desk.instrument,
			desk.price,
			desk.balance,
			desk.PositionStore,
			pair,
			decision,
		)

		position.onClose = func() {
			desk.positions.CompareAndDelete(decision.Symbol, position)

			desk.emitLifecycle(hindsight.LifecycleEvent{
				DecisionID: decision.ID,
				Symbol:     decision.Symbol,
				Kind:       "position_close",
				At:         time.Now().UTC(),
			})
		}

		// Keep the entry witness separate from each order's action identity.
		// Terminal execution facts are emitted before accounting; fill events
		// describe the resulting position transition.
		position.recordFill = func(kind string, execution kraken.ExecutionData) {
			desk.emitLifecycle(hindsight.LifecycleEvent{
				ActionCorrelationID: execution.ClientOrderID,
				DecisionID:          decision.ID,
				Symbol:              decision.Symbol,
				Kind:                kind,
				At:                  execution.Timestamp,
				Execution:           executionFact(execution),
			})
		}
		desk.positions.Store(decision.Symbol, position)

		if decision.Permit != nil && !decision.Permit() {
			desk.positions.CompareAndDelete(decision.Symbol, position)
			return &types.ExecutionRefusal{State: "stale", Detail: "candidate or authority changed before submission"}
		}
		_, err = position.Enter()

		if err != nil {
			desk.positions.CompareAndDelete(decision.Symbol, position)
			return err
		}

		desk.emitLifecycle(hindsight.LifecycleEvent{
			DecisionID: decision.ID,
			Symbol:     decision.Symbol,
			Kind:       "position_open",
			Action:     string(decision.Action),
			At:         time.Now().UTC(),
		})

	}

	return errnie.Error(err)
}

/*
SetLifecycleRecorder attaches the Hindsight trading-lifecycle sink. It is set
after construction so the live wiring can hand in the store without the desk
owning it; a nil recorder is the "Hindsight off" case and records nothing.
*/
func (desk *Desk) SetLifecycleRecorder(recorder LifecycleRecorder) {
	if desk == nil {
		return
	}

	if recorder == nil {
		desk.lifecycleRecorders = nil
		return
	}

	desk.lifecycleRecorders = []LifecycleRecorder{recorder}
}

/*
AddLifecycleRecorder registers an additional lifecycle sink, such as an
execution realization feedback bridge.
*/
func (desk *Desk) AddLifecycleRecorder(recorder LifecycleRecorder) {
	if desk == nil || recorder == nil {
		return
	}

	desk.lifecycleRecorders = append(desk.lifecycleRecorders, recorder)
}

func (desk *Desk) emitLifecycle(event hindsight.LifecycleEvent) {
	if desk == nil {
		return
	}

	for _, recorder := range desk.lifecycleRecorders {
		recorder.RecordLifecycle(event)
	}
}

/*
executionFact projects one venue execution record into the Hindsight fill fact
shape: every field the exchange reported, as strings so a zero decimal never
distinguishes itself from an absent one on the wire.
*/
func executionFact(execution kraken.ExecutionData) *hindsight.ExecutionFact {
	fact := &hindsight.ExecutionFact{
		OrderID:       execution.OrderID,
		ClientOrderID: execution.ClientOrderID,
		ExecID:        execution.ExecID,
		Side:          execution.Side,
		OrderStatus:   execution.OrderStatus,
		FillAt:        execution.Timestamp,
	}

	if execution.LastQty != nil {
		fact.LastQty = execution.LastQty.String()
	}

	if execution.LastPrice != nil {
		fact.LastPrice = execution.LastPrice.String()
	}

	if execution.CumQty != nil {
		fact.CumQty = execution.CumQty.String()
	}

	if execution.CumCost != nil {
		fact.CumCost = execution.CumCost.String()
	}

	if execution.AvgPrice != nil {
		fact.AvgPrice = execution.AvgPrice.String()
	}

	if execution.FeeUsdEquiv != nil {
		fact.FeeUsdEquiv = execution.FeeUsdEquiv.String()
	}

	return fact
}

/*
Close is retained for boot shutdown symmetry.
*/
func (desk *Desk) Close() error {
	desk.cancel()
	return nil
}

func (desk *Desk) Cancel() error {
	desk.positions.Range(func(key, value any) bool {
		position, ok := value.(*Position)

		if !ok || position == nil {
			return true
		}

		if err := position.Close(); err != nil {
			errnie.Error(errnie.Err(
				errnie.UnprocessableContent,
				"desk: could not cancel position",
				err,
			))
		}

		return true
	})

	return nil
}
