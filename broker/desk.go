package broker

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	"github.com/theapemachine/symm/system"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
)

/*
Desk is the broker Actor entrypoint and persistent position owner. It drains
market tickers and account executions from the shared thesis symbol queues and
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
	thesis         *types.Thesis
	equityObserver EquityObserver
	recorder       *audit.Recorder
	recovery       *Recovery
	perspective    PerspectiveReader
	positions      *sync.Map
	passage        *types.PassageModel
	balanceRefresh atomic.Bool
	maxPositions   int
	maxReserved    int
	// lifecycleRecorder is the optional Hindsight trading-lifecycle sink. It
	// records entry/fill/position-open/exit/close transitions observationally;
	// it is nil when Hindsight is not wired, and its failure never affects
	// trading progress.
	lifecycleRecorder LifecycleRecorder
	ObserveModule     func(string, time.Duration)
}

/*
LifecycleRecorder receives one real trading-lifecycle transition keyed by the
decision ID that caused it. Implementors own persistence; the desk only reports
and never blocks or fails the trade on it.
*/
type LifecycleRecorder interface {
	RecordLifecycle(event hindsight.LifecycleEvent)
}

/*
EquityObserver consumes each complete broker valuation after it is committed to
the shared thesis.
*/
type EquityObserver interface {
	Update(*types.Thesis, bool) error
}

/*
MarkObserver consumes executable position marks as predictive context. It is a
separate optional interface so broker valuation remains the only account-level
outcome while intra-position evidence can arrive at ticker cadence.
*/
type MarkObserver interface {
	ObserveMark(types.MarkFeedback) error
}

/*
NewDesk constructs the serial broker owner from its explicit dependencies. The
desk owns no transport or account objects itself: the caller supplies the API,
instrument, price, balance, thesis, recorder, and recovery handler it will
route through, so the live wiring and tests share one construction path rather
than a half-built struct. positions is the shared open-position map the recovery
publisher repopulates on boot. store persists position stoploss and thesis
checkpoint state; the desk embeds it so SaveThesis and Save resolve against a
live database rather than a nil pointer.
*/
func NewDesk(
	ctx context.Context,
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	thesis *types.Thesis,
	recorder *audit.Recorder,
	recovery *Recovery,
	store *PositionStore,
	positions *sync.Map,
	perspective PerspectiveReader,
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
		thesis:        thesis,
		recorder:      recorder,
		recovery:      recovery,
		PositionStore: store,
		perspective:   perspective,
		positions:     positions,
		passage:       types.NewPassageModel(),
		maxPositions:  viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:   viper.GetViper().GetInt("trading.slots.reserved"),
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
					desk.balance.Update()
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

// StepLevel3 routes one L3 frame to the symbol's open position, which folds it
// into its bounded resident liquidation state and derives the committed
// post-frame executable state for the guardian. Symbols with no open execution
// lifecycle perform no book work; the signal pipeline consumes the raw frame
// directly elsewhere.
func (desk *Desk) StepLevel3(level3 kraken.Level3Data) error {
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
ManualExit executes an operator override for one open symbol. The desk's
execution lock serializes it against entry admission and repeated override
clicks, while Position owns the regulator transition and market order.
*/
func (desk *Desk) ManualExit(symbol string) error {
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

	return position.ManualExit()
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
PublishEquity reports what the desk is worth if every open lot were closed now.

Cash alone understates the account while positions are open. Unrealized is the
profit/loss only; equity is cash plus the basis committed to open positions plus
that profit/loss.
*/
func (desk *Desk) PublishEquity() error {
	if desk == nil || desk.api == nil || desk.thesis == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"desk: API and thesis required",
			nil,
		))
	}

	tradeBalance, err := desk.api.TradeBalance()

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"desk: could not fetch trade balance",
			err,
		))
	}

	if err := desk.thesis.AppendEquity(tradeBalance); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"desk: could not publish account equity",
			err,
		))
	}

	// The equity observer is an optional downstream veto hook (e.g. a
	// whole-account regulator). Publishing equity to the thesis is the desk's
	// own responsibility and happens regardless; when no observer is wired the
	// valuation simply has no veto, rather than a fabricated no-op consumer.
	if desk.equityObserver == nil {
		return nil
	}

	if err := desk.equityObserver.Update(desk.thesis, desk.OpenPositions() > 0); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"desk: regulator rejected account equity",
			err,
		))
	}

	return nil
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
	case types.ActionEnter:
		if decision.ProposedQuantity == nil || decision.ProposedQuantity.Sign() <= 0 ||
			decision.Stoploss == nil || desk.price == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"desk: quantity, price, and strategy stoploss required for entry",
				nil,
			))
		}

		config := system.Cfg.Snapshot()

		if config == nil || config.Planner == nil {
			return errnie.Error(errnie.Err(
				errnie.Validation,
				"desk: planner configuration required",
				nil,
			))
		}

		found, loaded := desk.positions.Load(decision.Symbol)

		if loaded {
			position, valid := found.(*Position)

			if valid && position != nil && position.status() != types.CLOSED {
				return nil
			}
		}

		allocationClass, allocationErr := desk.entryAllocationClass(decision)

		if allocationErr != nil {
			return allocationErr
		}

		decision.AllocationClass = allocationClass

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
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: current entry cannot execute",
				costErr,
			))
		}

		feeRate := decimal.NewFromInt64(0).Add(fee.Fee).Div(
			decimal.NewFromInt64(100),
		)
		multiples := decision.Risk.Multiples

		if multiples.Risk <= 0 {
			multiples = types.DefaultRiskMultiples()
		}

		maxLoss := decision.Risk.MaxLoss

		if maxLoss == nil || maxLoss.Sign() <= 0 {
			maxLoss = decision.ProposedNotional
		}

		plan := types.NewRiskPlan(types.RiskInputs{
			ReferencePrice: cost.EntryPrice,
			Spread:         cost.Spread,
			Impact:         cost.Impact,
			TickSize:       &pair.TickSize,
			ExitFeeRate:    feeRate,
			EntryFeeRate:   feeRate,
			MaxLoss:        maxLoss,
			Multiples:      multiples,
		})

		if !plan.Present {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: current execution geometry cannot support the admitted risk plan",
				nil,
			))
		}

		if maximum := plan.MaxQuantity(cost.EntryPrice); maximum != nil &&
			maximum.Sign() > 0 && maximum.Cmp(decision.ProposedQuantity) < 0 {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: current risk budget no longer supports the admitted quantity",
				nil,
			))
		}

		stoploss, stopErr := types.NewStoplossWithPlan(
			desk.ctx,
			decision.Symbol,
			cost.EntryPrice,
			cost.BestBid,
			decision.Forecast,
			max(0, decision.ForecastHorizon),
			&pair.TickSize,
			feeRate,
			feeRate,
			&plan,
			time.Now(),
		)

		if stopErr != nil {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: current risk plan cannot construct an entry stop",
				stopErr,
			))
		}

		if decision.Stoploss != nil {
			_ = decision.Stoploss.Close()
		}

		decision.EntryCost = cost
		decision.ReferencePrice = decimal.NewFromInt64(0).Add(cost.BestAsk)
		decision.EntryPrice = decimal.NewFromInt64(0).Add(cost.EntryPrice)
		decision.Mark = decimal.NewFromInt64(0).Add(cost.BestBid)
		decision.ProposedNotional = decimal.NewFromInt64(0).Add(cost.GrossNotional).Add(
			cost.EntryFee,
		)
		decision.Risk = plan
		decision.Stoploss = stoploss
		decision.ExpectedReturn = nil
		decision.ExpectedFees = nil
		decision.ExpectedSpread = nil
		decision.ExpectedImpact = nil
		decision.PerspectiveReturn = 0
		decision.PerspectiveSources = nil
		decision.Utility = 0
		decision.OpportunityMargin = 0

		position := NewPosition(
			desk.ctx,
			desk.api,
			desk.instrument,
			desk.price,
			desk.balance,
			desk.recorder,
			desk.PositionStore,
			pair,
			decision,
			desk.perspective,
		)

		position.onClose = func() {
			desk.positions.CompareAndDelete(decision.Symbol, position)

			if desk.lifecycleRecorder != nil {
				desk.lifecycleRecorder.RecordLifecycle(hindsight.LifecycleEvent{
					DecisionID: decision.ID,
					Symbol:     decision.Symbol,
					Kind:       "position_close",
					At:         time.Now().UTC(),
				})
			}
		}

		// recordFill persists the authoritative venue fill (entry or exit) as a
		// decision-correlated Hindsight lifecycle event. It is observational and
		// never affects the position's transition itself.
		position.recordFill = func(kind string, execution kraken.ExecutionData) {
			if desk.lifecycleRecorder == nil {
				return
			}

			desk.lifecycleRecorder.RecordLifecycle(hindsight.LifecycleEvent{
				DecisionID: decision.ID,
				Symbol:     decision.Symbol,
				Kind:       kind,
				At:         execution.Timestamp,
				Execution:  executionFact(execution),
			})
		}
		desk.positions.Store(decision.Symbol, position)

		_, err = position.Enter()

		if err != nil {
			desk.positions.CompareAndDelete(decision.Symbol, position)
			return err
		}

		if desk.lifecycleRecorder != nil {
			desk.lifecycleRecorder.RecordLifecycle(hindsight.LifecycleEvent{
				DecisionID: decision.ID,
				Symbol:     decision.Symbol,
				Kind:       "position_open",
				Action:     string(decision.Action),
				At:         time.Now().UTC(),
			})
		}

	}

	return errnie.Error(err)
}

/*
MaxPositions is the number of concurrent positions the desk works under normal
conditions, before any reserve is drawn on.
*/
func (desk *Desk) MaxPositions() int {
	return desk.maxPositions
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

	desk.lifecycleRecorder = recorder
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
MaxReserved is the number of emergency slots that may only be used by a
strategy decision independently qualified for the reserve lane.
*/
func (desk *Desk) MaxReserved() int {
	if desk == nil {
		return 0
	}

	return desk.maxReserved
}

/*
OpenSlots reports independently occupied normal and reserve lanes while also
respecting the absolute four-position ceiling. A reserve position therefore
does not silently consume one of the two normal trading slots, and historical
over-capacity never reappears as fresh capacity.
*/
func (desk *Desk) OpenSlots(opportunity bool) int {
	if desk == nil {
		return 0
	}

	normalOpen, reserveOpen := desk.slotAvailability()

	if opportunity {
		return normalOpen + reserveOpen
	}

	return normalOpen
}

func (desk *Desk) entryAllocationClass(decision types.Decision) (string, error) {
	normalOpen, reserveOpen := desk.slotAvailability()

	switch decision.AllocationClass {
	case "none":
		return "", errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"desk: decision was not allocated a position slot",
			nil,
		))
	case "reserve":
		// Reserve capacity is a plain second capacity lane: it is allocated
		// upstream by expected economic outcome, never by legacy semantic
		// opportunity qualification.
		if reserveOpen > 0 {
			return "reserve", nil
		}

		if normalOpen > 0 {
			return "normal", nil
		}
	case "normal", "", "unallocated":
		if normalOpen > 0 {
			return "normal", nil
		}

		if reserveOpen > 0 {
			return "reserve", nil
		}
	default:
		return "", errnie.Error(errnie.Err(
			errnie.Validation,
			"desk: unknown allocation class",
			nil,
		))
	}

	return "", errnie.Error(errnie.Err(
		errnie.NotAcceptable,
		"desk: position capacity exhausted for requested allocation",
		nil,
	))
}

func (desk *Desk) slotAvailability() (int, int) {
	if desk == nil {
		return 0, 0
	}

	normal, reserve := desk.slotOccupancy()
	totalOpen := max(0, desk.maxPositions+desk.maxReserved-normal-reserve)
	normalOpen := min(max(0, desk.maxPositions-normal), totalOpen)
	reserveOpen := min(
		max(0, desk.maxReserved-reserve),
		max(0, totalOpen-normalOpen),
	)

	return normalOpen, reserveOpen
}

func (desk *Desk) slotOccupancy() (int, int) {
	if desk == nil || desk.positions == nil {
		return 0, 0
	}

	normal := 0
	reserve := 0
	desk.positions.Range(func(_, value any) bool {
		position, valid := value.(*Position)

		if !valid || position == nil {
			return true
		}

		if position.Decision.AllocationClass == "reserve" {
			reserve++
		} else {
			normal++
		}

		return true
	})

	return normal, reserve
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
