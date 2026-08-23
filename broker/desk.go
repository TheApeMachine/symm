package broker

import (
	"context"
	"iter"
	"sync"
	"sync/atomic"
	"time"

	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/spf13/viper"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/transport"
	"github.com/theapemachine/symm/system"
	wire "github.com/theapemachine/symm/telemetry/generated/telemetry"
	"github.com/theapemachine/symm/types"
	"golang.org/x/sync/errgroup"
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
	ui             *transport.MapReduce[*types.UIFrame]
	instrument     *Instrument
	price          *Price
	balance        *Balance
	thesis         *types.Thesis
	equityObserver EquityObserver
	recorder       *audit.Recorder
	recovery       *Recovery
	positions      *sync.Map
	passage        *types.PassageModel
	work           *transport.Consumer[*types.Symbol]
	balanceRefresh atomic.Bool
	executeMu      sync.Mutex
	maxPositions   int
	maxReserved    int
	ObserveModule  func(string, time.Duration)
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
NewDesk constructs the serial broker owner.
*/
func NewDesk(
	ctx context.Context,
	api *websocket.API,
	instrument *Instrument,
	price *Price,
	balance *Balance,
	thesis *types.Thesis,
	equityObserver EquityObserver,
	recorder *audit.Recorder,
	store *PositionStore,
	ui *transport.MapReduce[*types.UIFrame],
) *Desk {
	ctx, cancel := context.WithCancel(ctx)

	viper.SetDefault("trading.slots.normal", 2)
	viper.SetDefault("trading.slots.reserved", 2)

	desk := &Desk{
		ctx:            ctx,
		cancel:         cancel,
		status:         types.READY,
		api:            api,
		ui:             ui,
		instrument:     instrument,
		price:          price,
		balance:        balance,
		thesis:         thesis,
		equityObserver: equityObserver,
		recorder:       recorder,
		PositionStore:  store,
		positions:      &sync.Map{},
		passage:        types.NewPassageModel(),
		maxPositions:   viper.GetViper().GetInt("trading.slots.normal"),
		maxReserved:    viper.GetViper().GetInt("trading.slots.reserved"),
	}

	desk.recovery = NewRecovery(
		ctx, api, ui, instrument, price, balance, recorder, store, desk.positions,
	)
	desk.work = transport.NewConsumer[*types.Symbol](desk.Name(), desk.consume)
	thesis.Work(types.SourceDesk).Register(desk.work)

	if err := desk.recovery.Recover(); err != nil {
		desk.status = types.ERROR

		desk.err = errnie.Error(errnie.Err(
			errnie.Internal,
			"desk: failed to recover account positions",
			err,
		))

		return desk
	}

	return desk
}

func (desk *Desk) Name() string { return "desk" }

func (desk *Desk) Error() error { return desk.err }

func (desk *Desk) Cash() *decimal.Decimal {
	if desk == nil || desk.balance == nil {
		return nil
	}

	return desk.balance.Cash()
}

func (desk *Desk) consume() {
	go func() {
		defer func() {
			desk.thesis.Fail(desk.err)
		}()

		group, ctx := errgroup.WithContext(desk.ctx)
		group.SetLimit(types.ShardWorkers())

		for symbol := range desk.thesis.Work(types.SourceDesk).Drain(desk.work, nil) {
			select {
			case <-ctx.Done():
				desk.err = ctx.Err()
				return
			default:
			}

			if symbol == nil {
				continue
			}

			symbol := symbol
			group.Go(func() error {
				return desk.consumeSymbol(symbol)
			})
		}

		if err := group.Wait(); err != nil {
			desk.err = errnie.Error(err)
		}
	}()
}

func (desk *Desk) consumeSymbol(symbol *types.Symbol) error {
	drained := false

	for ticker := range symbol.MarketTickers(
		symbol.TickerConsumers[types.TickerConsumerDesk],
	) {
		drained = true
		desk.price.Update(&ticker)
		found, ok := desk.positions.Load(symbol.Symbol)

		if ok && found != nil {
			position, ok := found.(*Position)

			if ok && position != nil {
				position.onTicker(ticker)

				if observer, observesMarks := desk.equityObserver.(MarkObserver); observesMarks {
					err := observer.ObserveMark(position.MarkFeedback(ticker.Timestamp))

					if err != nil {
						return errnie.Error(err)
					}
				}
			}
		}
	}

	for execution := range symbol.MarketExecutions(
		symbol.ExecutionConsumers[types.ExecutionConsumerDesk],
	) {
		drained = true
		found, ok := desk.positions.Load(symbol.Symbol)

		if !ok || found == nil {
			continue
		}

		position, ok := found.(*Position)

		if !ok || position == nil {
			continue
		}

		if position.onExecution(kraken.Execution{
			Channel: "executions",
			Type:    "update",
			Data:    []kraken.ExecutionData{execution},
		}) {
			desk.foldPassage(position)
			desk.positions.CompareAndDelete(symbol.Symbol, position)
		}
	}

	if !drained {
		return nil
	}

	if desk.balanceRefresh.CompareAndSwap(false, true) {
		go func() {
			defer desk.balanceRefresh.Store(false)
			desk.balance.Update()
			err := desk.PublishEquity()

			if err != nil {
				desk.err = errnie.Error(err)
				desk.thesis.Fail(desk.err)
			}
		}()
	}

	return nil
}

/*
Status reports desk readiness.
*/
func (desk *Desk) Status() types.Status {
	return desk.status
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
			"desk: symbol is required for a manual exit",
			nil,
		)
	}

	desk.executeMu.Lock()
	defer desk.executeMu.Unlock()

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
	if desk == nil || desk.api == nil || desk.thesis == nil || desk.equityObserver == nil {
		return errnie.Error(errnie.Err(
			errnie.Validation,
			"desk: API, thesis, and equity observer required",
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

	if err := desk.equityObserver.Update(desk.thesis, desk.OpenPositions() > 0); err != nil {
		return errnie.Error(errnie.Err(
			errnie.UnprocessableContent,
			"desk: regulator rejected account equity",
			err,
		))
	}

	desk.thesis.Publish(&wire.FrameT{
		Type: wire.FrameEquityFrame,
		Value: &wire.EquityFrameT{
			Cash:       desk.balance.Cash().String(),
			Unrealized: tradeBalance.UnrealizedPnL.String(),
			Equity:     tradeBalance.Equity.String(),
		},
	})

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
PublishPositions sends all current non-terminal positions to the UI channel
so newly connected clients can see open positions immediately.
*/
func (desk *Desk) PublishPositions() {
	for position := range desk.Positions() {
		position.Publish()
	}
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
		desk.executeMu.Lock()
		defer desk.executeMu.Unlock()

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
				"desk: admission configuration required",
				nil,
			))
		}

		if result := config.Planner.Admission.Evaluate(decision); !result.Accepted {
			return errnie.Error(errnie.Err(
				errnie.NotAcceptable,
				"desk: entry no longer satisfies admission: "+result.Explanation(),
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

		if err = desk.SaveThesis(desk.thesis); err != nil {
			_ = stoploss.Close()
			return errnie.Error(errnie.Err(
				errnie.IO,
				"desk: checkpoint admitted entry",
				err,
			))
		}

		position := NewPosition(
			desk.ctx,
			desk.api,
			desk.ui,
			desk.instrument,
			desk.price,
			desk.balance,
			desk.recorder,
			desk.PositionStore,
			pair,
			decision,
		)

		// The exit snapshot belongs to the stoploss moment: the trigger is
		// the decision that closes the lot, so the thesis is checkpointed
		// exactly there, mirroring the entry checkpoint above.
		position.checkpoint = func() {
			if err := desk.SaveThesis(desk.thesis); err != nil {
				errnie.Error(errnie.Err(
					errnie.IO,
					"desk: checkpoint triggered exit",
					err,
				))
			}
		}

		desk.positions.Store(decision.Symbol, position)

		_, err = position.Enter()

		if err != nil {
			desk.positions.CompareAndDelete(decision.Symbol, position)
			return err
		}

		desk.thesis.Symbol(decision.Symbol).Positions.Push(position)
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
	reserveQualified := decision.Opportunity && decision.ReserveEligible

	switch decision.AllocationClass {
	case "none":
		return "", errnie.Error(errnie.Err(
			errnie.NotAcceptable,
			"desk: decision was not allocated a position slot",
			nil,
		))
	case "reserve":
		if reserveQualified && reserveOpen > 0 {
			return "reserve", nil
		}

		if normalOpen > 0 {
			return "normal", nil
		}
	case "normal", "", "unallocated":
		if normalOpen > 0 {
			return "normal", nil
		}

		if reserveQualified && reserveOpen > 0 {
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
