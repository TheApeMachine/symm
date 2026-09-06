package strategy

import (
	"math/big"
	"time"

	"sync/atomic"

	"github.com/google/uuid"
	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
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
	MaximumCost *big.Rat
	Allowed     *atomic.Bool

	CorrelationID string
	Symbol        string
	At            time.Time
	MarketAt      time.Time
	Kind          types.Action
	Reduce        bool
	Quantity      *big.Rat
	Reference     *big.Rat
	Mode          Mode
	Skill         SkillReading
}

/*
ExecutionDesk is the account the policy lane reaches. It is deliberately
narrow: the agent decides what to do and how much, and the desk owns venue
mechanics. The agent never constructs venue orders itself, and a learning-only
run leaves this nil so no code path can produce one.

Submit must not talk to the venue. It is called from the workspace consumer
that also feeds the terminal, so a round-trip taken inside it stops the whole
pipeline for its duration. Implementations queue the intent and place it
elsewhere.
*/
type ExecutionDesk interface {
	Submit(ExecutionIntent) error
}

/*
RealizationFeedback connects an execution desk's asynchronous placement and fill
feedback to the agent's realization circuit breaker.
*/
type RealizationFeedback interface {
	AttachRealization(*RealizationMeter)
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

/*
ExecutionReporter is an account that can describe its own outcomes. It is
optional: an agent whose desk cannot report simply shows nothing rather than
inventing a status.
*/
type ExecutionReporter interface {
	Execution() ExecutionStatus
}

/* Execution owns entry authority, account dispatch and its observed failures. */
type Execution struct {
	Candidates *CandidateBook
	allowed    atomic.Bool

	Skill         *SkillMeter
	Desk          ExecutionDesk
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
func (execution *Execution) SetExecution(desk ExecutionDesk, account Account) {
	execution.allowed.Store(false)
	execution.Desk = desk

	if desk == nil {
		account = AccountNone
	}
	execution.Skill = NewSkillMeter(account, time.Now())
	execution.Realization = NewRealizationMeter()

	if feedback, ok := desk.(RealizationFeedback); ok {
		feedback.AttachRealization(execution.Realization)
	}
}

/* Propose records non-exploring local increases as candidates, independent of Skill. */
func (execution *Execution) Propose(local *LocalLearning, market *learningMarket, action LearningAction, requested *big.Rat,
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
	quantity, gross := lane.wallet.pricing.Sweep(book, requested, &lane.wallet.cash, true, nil, nil)

	if quantity.Cmp(requested) != 0 {
		return nil
	}
	cost := lane.wallet.pricing.Total(new(big.Rat), gross, true)
	record := hindsight.CandidateRecord{ID: uuid.NewString(), Decision: identity, Symbol: market.symbol,
		Action: string(action.Kind), Power: action.Power, At: market.at, MarketAt: marketAt, Capture: market.capture,
		GridVersion: market.gridVersion, Context: append([]uint64(nil), market.context...),
		Scope: reading.Scope, Global: reading.Global, SymbolPrior: reading.Symbol, Prior: reading.Selected,
		Quantity: requested.RatString(), Notional: cost.RatString(), Reference: book.Asks.Low.Price.Rat().RatString(),
		Horizon: market.horizon(), QtyMinimum: lane.wallet.pricing.Minimum.RatString(), QtyIncrement: lane.wallet.pricing.Lot.RatString(),
		CostMinimum: lane.wallet.pricing.CostMinimum.RatString(), FeeRate: lane.wallet.pricing.Rate.RatString()}
	record.Authority = market.authority

	for _, token := range market.sequence {
		record.Quantities = append(record.Quantities, local.Grid.Columns[token-1])
	}

	if account, ok := execution.Desk.(ExecutionAccount); ok {
		state := account.Account()
		record.AccountCash, record.AccountVersion = state.Cash, state.Mark.Version
		record.AccountEquity = decimal.NewFromFloat64(state.Mark.Equity).String()
	}
	candidate := &EntryCandidate{Record: record, action: action, quantity: new(big.Rat).Set(requested), cost: cost, bid: book.Bids.High.Price.Rat()}
	lane.wallet.pricing.Sweep(book, requested, &lane.wallet.cash, true, &candidate.ladder, nil)
	candidate.Intent = ExecutionIntent{CorrelationID: record.ID, Symbol: market.symbol, At: market.at, MarketAt: marketAt,
		Kind: action.Kind, Quantity: new(big.Rat).Set(requested), Reference: book.Asks.Low.Price.Rat(),
		Mode: execution.Mode(), Skill: execution.Skill.Reading(), Candidate: candidate, MaximumCost: cost, Allowed: &execution.allowed}

	if err := execution.Candidates.Publish(candidate); err != nil {
		return err
	}
	// Link later local outcomes to this prospective candidate; the decision input stays immutable.
	lane.trace[len(lane.trace)-1].candidateID = record.ID
	market.events[len(market.events)-1].CandidateID = record.ID
	return nil
}

/* Submit hands an already selected intent to the asynchronous account boundary. */
func (execution *Execution) Submit(intent ExecutionIntent) error {
	if execution.Desk == nil || (!intent.Reduce && execution.Mode() != ModeTrading) {
		intent.Allocation.Report(hindsight.AllocationResult{State: "aborted", At: time.Now().UTC(), Detail: "execution authority or account unavailable at dispatch"})
		return nil
	}

	if err := execution.Desk.Submit(intent); err != nil {
		intent.Allocation.Report(hindsight.AllocationResult{State: "aborted", At: time.Now().UTC(), Detail: err.Error()})
		execution.rejected++
		execution.lastRejection = errnie.Error(errnie.Err(errnie.IO, "policy intent was not accepted by the account", err))
		return nil
	}
	execution.dispatched++
	return nil
}

/* Reduce re-evaluates existing account inventory even when the virtual policy is flat or demoted. */
func (execution *Execution) Reduce(local *LocalLearning, market *learningMarket, book *spotbook.Book) error {
	account, ok := execution.Desk.(ExecutionAccount)

	if !ok {
		return nil
	}
	state := account.Account()
	amount, found := state.Positions[market.symbol]

	if !found {
		return nil
	}
	quantity, valid := new(big.Rat).SetString(amount)

	if !valid {
		panic("execution: malformed authoritative inventory")
	}

	if quantity.Sign() <= 0 {
		return nil
	}
	wallet := virtualWallet{}
	if err := wallet.initialize(local.initial, local.pair(market.symbol), local.fee(market.symbol).Fee); err != nil {
		return err
	}
	wallet.cash.SetInt64(0)
	wallet.quantity.Set(quantity)
	context := wallet.context(market.sequence, book, state.Mark.Equity, nil)
	action, _, err := local.Knowledge.Select(market.symbol, context, wallet.actions(book, nil), false)

	if err != nil {
		return err
	}

	if !action.Reduce {
		return nil
	}
	requested := wallet.request(book, action, 1, nil)
	return execution.Submit(ExecutionIntent{CorrelationID: uuid.NewString(), Symbol: market.symbol, At: market.at,
		Kind: action.Kind, Reduce: true, Quantity: requested, Reference: book.Bids.High.Price.Rat(), Mode: execution.Mode(), Skill: execution.Skill.Reading()})
}
