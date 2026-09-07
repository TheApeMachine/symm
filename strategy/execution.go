package strategy

import (
	"errors"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/kraken"
	"github.com/theapemachine/symm/kraken/websocket"
	"strconv"
	"sync"
	"time"

	"sync/atomic"

	"github.com/google/uuid"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/types"
)

/*
ExecutionIntent is one policy decision expressed in account terms, ready for a
venue. Quantity is the amount the policy lane fixed against the same displayed
book it was measured on, in the instrument's own precision.

Reduce distinguishes a decision that sells inventory from one that acquires
it. Mode records the authority under which the intent was produced, so an
order can never be attributed to a lower authority than the one that made it.
*/
type ExecutionIntent struct {
	Candidate   *EntryCandidate
	PortfolioID string
	Allocation  *AllocationReceipt
	MaximumCost *decimal.Decimal
	Allowed     *atomic.Bool

	CorrelationID string
	Symbol        string
	At            time.Time
	MarketAt      time.Time
	Kind          types.Action
	Reduce        bool
	Quantity      *decimal.Decimal
	Reference     *decimal.Decimal
	Mode          Mode
	Skill         SkillReading
}

/*
ExecutionStatus is what the account did with the agent's intents. Because
placement happens off the deciding path, an outcome is a report rather than a
return value, and every category here is a distinct fact: Diverged means the
account disagreed with the simulated wallet, Dropped means the venue was slower
than the agent was deciding, and Failed means an order was actually refused.
*/
type ExecutionStatus struct {
	Refused     uint64 `json:"refused"`
	LastRefusal string `json:"lastRefusal,omitempty"`
	Submitted   uint64 `json:"submitted"`
	Unsupported uint64 `json:"unsupported"`
	Diverged    uint64 `json:"diverged"`
	Dropped     uint64 `json:"dropped"`
	Failed      uint64 `json:"failed"`
	Queued      int    `json:"queued"`
	LastFailure string `json:"lastFailure,omitempty"`
}

/* Execution owns entry authority, account dispatch and its observed failures. */
type Execution struct {
	Candidates *CandidateBook
	allowed    atomic.Bool

	Skill         *SkillMeter
	API           *websocket.API
	Balance       *broker.Balance
	Positions     sync.Map
	price         *broker.Price
	record        func(hindsight.LifecycleEvent) error
	inFlight      sync.Map
	failures      atomic.Uint64
	submissions   atomic.Uint64
	refusals      atomic.Uint64
	durability    atomic.Pointer[error]
	Realization   *RealizationMeter
	dispatched    uint64
	rejected      uint64
	lastRejection error
}

/* Mode is effective increase authority; it does not prohibit liquidation. */
func (execution *Execution) Mode() Mode {
	if execution.Skill == nil || execution.Skill.Mode() != ModeTrading || !execution.Realization.AllowsTrading() {
		return ModeLearning
	}
	return ModeTrading
}

/* Refresh revokes prospective claims when effective increase authority changes. */
func (execution *Execution) Refresh(at time.Time) error {
	allowed := execution.Mode() == ModeTrading

	if execution.allowed.Swap(allowed) == allowed || execution.Candidates == nil {
		return nil
	}
	for symbol := range execution.Candidates.current {
		if err := execution.Candidates.Invalidate(symbol, at, "increase authorization changed"); err != nil {
			return err
		}
	}
	return nil
}

/* SetExecution attaches one account and starts live competence cold. */
func (execution *Execution) SetExecution(
	api *websocket.API,
	price *broker.Price,
	balance *broker.Balance,
	account Account,
	record func(hindsight.LifecycleEvent) error,
) error {
	execution.allowed.Store(false)
	execution.API = api
	execution.price = price
	execution.Balance = balance
	execution.record = record
	execution.Skill = NewSkillMeter(account, time.Now())
	execution.Realization = NewRealizationMeter()
	recovery := position.Recovery{API: api, Price: price}
	positions, err := recovery.Recover(api.Context(), balance.Quote, execution.observe)

	for symbol, regulator := range positions {
		execution.Positions.Store(symbol, regulator)
	}

	if err != nil {
		execution.durability.Store(&err)
	}
	return err
}

/* Propose records non-exploring local increases as candidates, independent of Skill. */
func (execution *Execution) Propose(local *LocalLearning, market *learningMarket, action LearningAction, requested *decimal.Decimal,
	book *spotbook.Book, marketAt time.Time, identity uint64, reading KnowledgeReading) error {
	if action.Reduce {
		return nil
	} // Account reductions are selected from authoritative inventory in Reduce.

	if execution.Candidates == nil {
		return nil
	}

	if err := execution.Candidates.Invalidate(market.symbol, market.at, "local policy changed"); err != nil {
		return err
	}

	if action.Kind == types.ActionHold || requested == nil || requested.Sign() <= 0 || market.horizon() <= 0 {
		return nil
	}
	lane := &market.lanes[len(market.lanes)-1]
	quantity, gross, err := local.price.Sweep(book, requested, lane.wallet.cash, broker.BUY, nil, nil)

	if err != nil {
		return err
	}

	if quantity.Cmp(requested) != 0 {
		return nil
	}
	cost := local.price.WithFee(market.symbol, gross, broker.BUY)
	record := hindsight.CandidateRecord{ID: uuid.NewString(), Decision: identity, Symbol: market.symbol,
		Action: string(action.Kind), Power: action.Power, At: market.at, MarketAt: marketAt, Capture: market.capture,
		GridVersion: market.gridVersion, Context: append([]uint64(nil), market.context...),
		Scope: reading.Scope, Global: reading.Global, SymbolPrior: reading.Symbol, Prior: reading.Selected,
		Quantity: requested.String(), Notional: cost.String(), Reference: book.Asks.Low.Price.String(),
		Horizon: market.horizon()}
	record.Authority = market.authority

	for _, token := range market.sequence {
		record.Quantities = append(record.Quantities, local.Grid.Columns[token-1])
	}

	if execution.Balance != nil {
		state := execution.Account()
		record.AccountCash, record.AccountVersion = state.Cash, state.Mark.Version
		record.AccountEquity = decimal.NewFromFloat64(state.Mark.Equity).String()
	}
	candidate := &EntryCandidate{Record: record, action: action, quantity: requested, cost: cost, bid: book.Bids.High.Price}
	if _, _, err := local.price.Sweep(book, requested, lane.wallet.cash, broker.BUY, &candidate.ladder, nil); err != nil {
		return err
	}
	candidate.Intent = ExecutionIntent{CorrelationID: record.ID, Symbol: market.symbol, At: market.at, MarketAt: marketAt,
		Kind: action.Kind, Quantity: requested, Reference: book.Asks.Low.Price,
		Mode: execution.Mode(), Skill: execution.Skill.Reading(), Candidate: candidate, MaximumCost: cost, Allowed: &execution.allowed}

	if err := execution.Candidates.Publish(candidate); err != nil {
		return err
	}
	// Link later local outcomes to this prospective candidate; the decision input stays immutable.
	lane.trace[len(lane.trace)-1].candidateID = record.ID
	market.events[len(market.events)-1].CandidateID = record.ID
	return nil
}

/* Submit queues the policy action directly on its position's guardian. */
func (execution *Execution) Submit(intent ExecutionIntent) error {
	if execution.API == nil || (!intent.Reduce && execution.Mode() != ModeTrading) {
		intent.Allocation.Report(hindsight.AllocationResult{
			State:  "aborted",
			At:     time.Now().UTC(),
			Detail: "execution authority or account unavailable at dispatch",
		})
		return nil
	}
	value, found := execution.Positions.Load(intent.Symbol)

	if !found {
		if intent.Reduce {
			return errnie.Error(errnie.Err(
				errnie.NotFound, "execution: no position for "+intent.Symbol, nil,
			))
		}
		value = position.NewRegulator(
			execution.API.Context(), execution.API, execution.price,
			intent.Symbol, execution.observe,
		)
		execution.Positions.Store(intent.Symbol, value)
	}
	regulator := value.(*position.Regulator)
	err := regulator.Guardian.Publish(func() error {
		if err := execution.admit(intent); err != nil {
			intent.Allocation.Report(hindsight.AllocationResult{
				State: "aborted", At: time.Now().UTC(), Detail: err.Error(),
			})
			execution.refusals.Add(1)
			return err
		}
		side := broker.BUY

		if intent.Reduce {
			side = broker.SELL
		}
		execution.inFlight.Store(intent.CorrelationID, intent)
		err := regulator.Submit(intent.CorrelationID, side, intent.Quantity)
		execution.Realization.ObserveSubmission(err)

		if err != nil {
			execution.failures.Add(1)
			execution.Balance.Release(intent.CorrelationID, time.Time{})
			execution.inFlight.Delete(intent.CorrelationID)
			intent.Allocation.Report(hindsight.AllocationResult{
				State: "aborted", At: time.Now().UTC(), Detail: err.Error(),
			})
			return err
		}
		execution.submissions.Add(1)
		intent.Allocation.Report(hindsight.AllocationResult{
			State: "submitted", At: time.Now().UTC(),
		})
		return nil
	})

	if err != nil {
		execution.rejected++
		execution.lastRejection = err
		return err
	}
	execution.dispatched++
	return nil
}

/* admit checks current authority and economics immediately before submission. */
func (execution *Execution) admit(intent ExecutionIntent) error {
	if intent.Reduce {
		return nil
	}

	if !execution.allowed.Load() || !execution.Realization.AllowsTrading() || execution.durability.Load() != nil {
		return errnie.Error(&types.ExecutionRefusal{
			State:  "authorization blocked",
			Detail: "entry authority or persistence is unavailable",
		})
	}

	if intent.Candidate == nil {
		return errnie.Error(&types.ExecutionRefusal{
			State: "no longer executable", Detail: "prospective candidate required",
		})
	}

	if _, state := intent.Candidate.Reprice(
		execution.API, execution.price, time.Now().UTC(),
	); state != "" {
		return errnie.Error(&types.ExecutionRefusal{
			State: state, Detail: "candidate economics no longer hold",
		})
	}

	if !execution.Balance.Reserve(intent.CorrelationID, intent.MaximumCost) {
		return errnie.Error(&types.ExecutionRefusal{
			State: "insufficient capital", Detail: "available quote cash cannot fund commitment",
		})
	}
	return nil
}

/* observe delivers the original venue fact to learning and the shared journal. */
func (execution *Execution) observe(fill kraken.ExecutionData) error {
	kind := "execution_fill"
	terminal := false

	switch fill.OrderStatus {
	case "filled", "iceberg_filled", "canceled", "expired", "rejected":
		kind, terminal = "execution_terminal", true
	}
	event := hindsight.LifecycleEvent{
		ActionCorrelationID: fill.ClientOrderID,
		Symbol:              fill.Symbol,
		Kind:                kind,
		At:                  fill.Timestamp,
		Execution:           &fill,
	}
	value, found := execution.inFlight.Load(fill.ClientOrderID)

	if found {
		intent := value.(ExecutionIntent)
		intent.Allocation.Observe(event)

		if terminal && fill.AvgPrice != nil && fill.CumQty != nil && fill.CumQty.Sign() > 0 {
			execution.Realization.ObserveFill(
				intent.Reference.Float64(), fill.AvgPrice.Float64(), intent.Reduce,
			)
		}
	}

	if terminal {
		execution.Balance.Release(fill.ClientOrderID, fill.Timestamp)
		execution.inFlight.Delete(fill.ClientOrderID)
	}

	if err := execution.record(event); err != nil {
		execution.durability.Store(&err)
		return err
	}
	return nil
}

/* Account projects the current balance into the capital learner's inputs. */
func (execution *Execution) Account() AccountState {
	reading := execution.Balance.Reading.Load()

	if reading == nil {
		return AccountState{Reason: "awaiting authoritative account mark"}
	}
	state := AccountState{
		ActualCash: reading.Cash,
		Positions:  reading.Positions,
		Complete:   reading.Complete,
		Reason:     reading.FundingReason,
		Mark:       EquityMark{At: reading.At, Version: reading.Version},
	}
	cash, err := decimal.NewFromString(reading.AvailableCash)

	if err != nil {
		panic(errnie.Error(errnie.Err(errnie.Validation, "account: malformed cash", err)))
	}
	committed := execution.Balance.Committed()
	state.Cash, state.Committed = cash.Sub(committed).String(), committed.String()
	state.Mark.Equity, err = strconv.ParseFloat(reading.Equity, 64)

	if err != nil {
		panic(errnie.Error(errnie.Err(errnie.Validation, "account: malformed equity", err)))
	}

	if reading.NetFunding != "" {
		state.Mark.NetFunding, err = strconv.ParseFloat(reading.NetFunding, 64)

		if err != nil {
			panic(errnie.Error(errnie.Err(errnie.Validation, "account: malformed funding", err)))
		}
		state.Mark.HasFunding = true
	}
	state.Complete = state.Complete && state.Mark.HasFunding
	return state
}

/* Stats reports actual asynchronous submission outcomes. */
func (execution *Execution) Stats() ExecutionStatus {
	return ExecutionStatus{
		Submitted: execution.submissions.Load(),
		Failed:    execution.failures.Load(),
		Refused:   execution.refusals.Load(),
	}
}

/* Reduce re-evaluates existing account inventory even when the virtual policy is flat or demoted. */
func (execution *Execution) Reduce(local *LocalLearning, market *learningMarket, book *spotbook.Book) error {
	if execution.Balance == nil {
		return nil
	}
	state := execution.Account()
	amount, found := state.Positions[market.symbol]

	if !found {
		return nil
	}
	quantity, err := decimal.NewFromString(amount)

	if err != nil {
		return errnie.Error(errnie.Err(errnie.Validation, "execution: malformed inventory", err))
	}

	if quantity.Sign() <= 0 {
		return nil
	}
	wallet := virtualWallet{}
	if err := wallet.initialize(local.initial, local.price, market.symbol); err != nil {
		return err
	}
	wallet.cash = zero
	wallet.quantity = quantity
	context := wallet.context(market.sequence, book, state.Mark.Equity, nil)
	actions, err := wallet.actions(book, nil)

	if err != nil {
		return err
	}
	action, _, err := local.Knowledge.Select(market.symbol, context, actions, false)

	if err != nil {
		return err
	}

	if !action.Reduce {
		return nil
	}
	requested, err := wallet.request(book, action, 1, nil)

	if err != nil {
		return err
	}
	return execution.Submit(ExecutionIntent{CorrelationID: uuid.NewString(), Symbol: market.symbol, At: market.at,
		Kind: action.Kind, Reduce: true, Quantity: requested, Reference: book.Bids.High.Price, Mode: execution.Mode(), Skill: execution.Skill.Reading()})
}

/* Close stops publication and drains accepted position events before shutdown. */
func (execution *Execution) Close() error {
	var err error
	guardians := make([]*position.Guardian, 0)
	execution.Positions.Range(func(_, value any) bool {
		guardian := value.(*position.Regulator).Guardian
		err = errors.Join(err, guardian.Close())
		guardians = append(guardians, guardian)
		return true
	})

	for _, guardian := range guardians {
		<-guardian.Done
	}
	return errnie.Error(err)
}
