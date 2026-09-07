package strategy

import (
	"math"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/*
learningExperience retains one unresolved decision's economic starting reference,
account state, and the precursor context that was active when it was issued.
*/
type learningExperience struct {
	id           uint64
	candidateID  string
	action       LearningAction
	wealthBefore float64
	accountState string
	authority    float64
	at           time.Time
	tokens       []uint64
	context      []uint64
	reading      KnowledgeReading
	count        int
}

/* learningLane owns execution, elapsed-time accounting, and unresolved actions. */
type learningLane struct {
	wallet                  virtualWallet
	paper                   bool
	ledger                  AccountReward
	outcome                 learning.RewardOutcome
	version                 uint64
	pending                 uint64
	action                  LearningAction
	requested               *decimal.Decimal
	trace                   []learningExperience
	equity                  float64
	complete                bool
	issued, fills, resolved uint64
	episodes                uint64
	realized, spent         float64
	exhausted               bool
	lastPrior               learning.PriorReading
}

/*
settle resolves decisions upon causal lifecycle transitions: when a new Impulse
state arrives (changed == true), the position exits, or the account recycles.
Decisions are never scored over an arbitrary fixed time window.
*/
func (lane *learningLane) settle(
	local *LocalLearning, market *learningMarket, index int, marketAt time.Time, changed bool,
) error {
	if !changed || len(lane.trace) == 0 {
		return nil
	}

	if err := lane.resolve(local, market, index, marketAt, lane.trace, false); err != nil {
		return err
	}

	lane.trace = lane.trace[:0]
	return nil
}

/*
resolve incorporates each decision's realized economic transition into the model:
log wealth growth (ln(W_after / W_before)) and elapsed time are preserved separately.
*/
func (lane *learningLane) resolve(
	local *LocalLearning,
	market *learningMarket,
	index int,
	marketAt time.Time,
	due []learningExperience,
	truncated bool,
) error {
	for _, experience := range due {
		elapsed := market.at.Sub(experience.at)

		if elapsed <= 0 {
			elapsed = time.Millisecond
		}

		reading, err := local.Knowledge.Resolve(
			experience, experience.wealthBefore, lane.equity, elapsed,
		)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[agent] failed to resolve action",
				err,
			))
		}

		prior := reading.Prior()

		if lane.paper && local.execution.Skill != nil && !truncated &&
			!experience.at.Before(local.execution.Skill.window) {

			local.execution.Skill.window = market.at
			skillTarget := 0.0

			if experience.wealthBefore > 0 {
				skillTarget = (lane.equity - experience.wealthBefore) / experience.wealthBefore
			}

			local.execution.Skill.Observe(skillTarget, experience.authority, market.at)

			if err := local.execution.Refresh(market.at); err != nil {
				return err
			}
		}

		lane.lastPrior = prior
		lane.resolved++
		local.resolved++

		event := lane.event(market, index, "resolved", experience.id, marketAt)
		event.CandidateID = experience.candidateID
		event.Context = append([]uint64(nil), experience.context...)
		growth := 0.0

		if experience.wealthBefore > 0 && lane.equity > 0 {
			growth = math.Log(lane.equity / experience.wealthBefore)
		}

		event.AbsoluteSkillTarget = &growth
		event.Scope, event.GlobalPrior, event.SymbolPrior = experience.reading.Scope, experience.reading.Global, experience.reading.Symbol
		event.Authority = experience.authority
		event.Action, event.Power, event.Reduce = string(experience.action.Kind), experience.action.Power, experience.action.Reduce
		event.TargetUnit = "compounded_growth"
		event.Target = reading.Rate
		event.Prior = prior
		event.Profit = lane.outcome.TotalReward
		event.Authorized, event.Truncated = local.execution.Mode().String(), truncated
		market.events = append(market.events, event)
	}

	return nil
}

/*
recycle restarts a lane that can no longer act because its capital was exhausted.
Outstanding decisions resolve against realized equity and a new episode begins.
*/
func (lane *learningLane) recycle(
	local *LocalLearning, market *learningMarket, index int, book *spotbook.Book, marketAt time.Time,
) error {
	if lane.wallet.quantity.Sign() != 0 || lane.pending != 0 {
		lane.exhausted = false
		return nil
	}

	maximum, err := lane.wallet.maximum(book, true)

	if err != nil {
		return err
	}

	if local.price.Tradable(market.symbol, maximum, book.BestAsk().Price) {
		lane.exhausted = false
		return nil
	}

	lane.exhausted = true

	if err := lane.resolve(local, market, index, marketAt, lane.trace, true); err != nil {
		return err
	}

	lane.trace = lane.trace[:0]
	lane.realized += lane.equity - local.initial.Float64()
	spent := lane.wallet.restart(local.initial).Float64()
	lane.spent += spent
	lane.episodes++
	lane.ledger = AccountReward{}
	lane.equity = local.initial.Float64()
	outcome, err := lane.ledger.Measure(EquityMark{
		At: market.at, Version: lane.version, Equity: lane.equity, HasFunding: true,
	})

	if err != nil {
		return err
	}

	lane.outcome = outcome
	lane.action = LearningAction{}

	event := lane.event(market, index, "recycled", lane.episodes, marketAt)
	event.Authorized = local.execution.Mode().String()
	market.events = append(market.events, event)
	return nil
}

/*
issue conditions the next decision on the temporal precursor context and the
account's own state (flat vs holding). Independent exploratory lanes explore
counterfactual actions; the policy lane selects the best supported action.
*/
func (lane *learningLane) issue(
	local *LocalLearning, market *learningMarket, index int, book *spotbook.Book, marketAt time.Time,
) error {
	accountState := lane.wallet.state()
	market.context = market.PrecursorContext()

	var err error
	market.actions, err = lane.wallet.actions(book, market.actions)

	if err != nil {
		return err
	}

	explore := !lane.paper
	action, reading, err := local.Knowledge.Select(
		market.symbol, accountState, market.context, market.actions, explore,
	)

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[agent] failed to select action",
			err,
		))
	}

	// Exploratory lanes balance counterfactual actions across the feasible set.
	if !lane.paper && len(market.actions) > 0 {
		candidateAction := market.actions[index%len(market.actions)]
		action = candidateAction
		reading = local.Knowledge.Reading(market.symbol, accountState, market.context, action)
	}

	prior := reading.Selected
	selectedAction := action
	influence, authority := 1.0, market.authority

	if lane.paper {
		influence = prior.Authority

		if action.Kind != types.ActionHold && !action.Reduce && reading.Economic.Rate <= 0 {
			action = LearningAction{Kind: types.ActionHold}
		}
	}

	if len(lane.trace) != 0 && action == lane.action {
		return nil
	}

	requested, err := lane.wallet.request(book, action, influence)

	if err != nil {
		return err
	}

	price := book.Asks.Low.Price

	if action.Reduce {
		price = book.Bids.High.Price
	}

	if action.Kind != types.ActionHold && !local.price.Tradable(market.symbol, requested, price) {
		action = LearningAction{Kind: types.ActionHold}
		requested = zero
	}

	if action != selectedAction {
		reading = local.Knowledge.Reading(market.symbol, accountState, market.context, action)
		prior = reading.Selected
	}

	identity, err := local.Knowledge.Issue(
		market.symbol, accountState, market.context, action, authority,
	)

	if err != nil {
		return err
	}

	lane.pending, lane.action, lane.requested = identity, action, requested
	experience := learningExperience{
		id:           identity,
		action:       action,
		at:           market.at,
		wealthBefore: lane.equity,
		accountState: accountState,
		authority:    authority,
		reading:      reading,
	}
	experience.tokens = append([]uint64(nil), market.currentConditions...)
	experience.count = len(experience.tokens)
	experience.context = append([]uint64(nil), market.context...)
	lane.trace = append(lane.trace, experience)
	lane.issued++
	local.decisions++

	event := lane.event(market, index, "issued", identity, marketAt)
	event.Context = append([]uint64(nil), market.context...)
	event.Scope, event.GlobalPrior, event.SymbolPrior = reading.Scope, reading.Global, reading.Symbol
	event.BaselineRate = lane.outcome.Rate

	for _, token := range market.currentConditions {
		rawID := int(token & 0xFFFF)

		if rawID > 0 && rawID <= len(local.Grid.Columns) {
			event.Quantities = append(event.Quantities, local.Grid.Columns[rawID-1])
		}
	}

	event.GridVersion, event.Authority, event.Quantity, event.Prior = market.gridVersion, authority, requested.String(), prior
	event.Authorized = local.execution.Mode().String()
	market.events = append(market.events, event)

	if lane.paper {
		return local.execution.Propose(local, market, action, requested, book, marketAt, identity, reading)
	}

	return nil
}

/* event freezes one small decision boundary for the durable learning journal. */
func (lane *learningLane) event(
	market *learningMarket,
	index int,
	kind string,
	identity uint64,
	marketAt time.Time,
) hindsight.LearningEvent {
	mode := "virtual"

	if lane.paper {
		mode = "policy"
	}

	cash := ""

	if lane.wallet.cash != nil {
		cash = lane.wallet.cash.String()
	}

	inventory := ""

	if lane.wallet.quantity != nil {
		inventory = lane.wallet.quantity.String()
	}

	return hindsight.LearningEvent{
		ID:        identity,
		Symbol:    market.symbol,
		Capture:   market.capture,
		Lane:      index,
		Mode:      mode,
		Kind:      kind,
		At:        market.at,
		MarketAt:  marketAt,
		Action:    string(lane.action.Kind),
		Power:     lane.action.Power,
		Reduce:    lane.action.Reduce,
		Cash:      cash,
		Inventory: inventory,
		Profit:    lane.outcome.TotalReward,
		Episode:   lane.episodes,
		Complete:  lane.complete,
		ValuedAt:  lane.outcome.Through.At,
	}
}
