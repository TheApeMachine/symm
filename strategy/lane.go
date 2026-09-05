package strategy

import (
	"math/big"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/*
horizonEpochs is how many impulse epochs of forward tape one decision is
measured over. The epoch itself is measured from this market's own cadence of
impulse changes, so the horizon adapts to the instrument; the multiple is a
declared operating choice and is reported with every view.

Every decision — waiting included — is scored over this same forward window.
An earlier design resolved a waiting decision on the very next book update
because the wallet happened to be flat, while an entry was only scored once
its position closed. Those are not comparable measurements, and the shorter
window made waiting look reliably harmless.
*/
const horizonEpochs = 8

/*
learningExperience retains one unresolved decision's return reference and the
impulse that was hot when it issued. The tokens are kept so a resolved outcome
can be credited back to the quantities that were present, and the authority is
the one fixed at issue time — never the one visible when the outcome arrives.
*/
type learningExperience struct {
	id          uint64
	candidateID string
	action      LearningAction
	value, rate float64
	authority   float64
	at          time.Time
	tokens      []uint64
	context     []uint64
	horizon     time.Duration
	reading     KnowledgeReading
	count       int
}

/* learningLane owns execution, elapsed-time accounting and unresolved actions. */
type learningLane struct {
	wallet                  virtualWallet
	paper                   bool
	ledger                  AccountReward
	outcome                 learning.RewardOutcome
	version                 uint64
	pending                 uint64
	action                  LearningAction
	requested               *big.Rat
	ladder                  depthLadder
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
settle resolves every decision whose measurement window has closed, using the
account's current executable valuation. Waiting and trading are scored over
the same window: the target is the account's change since the decision issued,
less the elapsed-time cost at the rate known then, over its starting capital.

Overlapping windows are correlated, and the account ledger still counts each
economic change once. This assigns each decision a forward return; it does not
claim each decision independently caused the whole of it.
*/
func (lane *learningLane) settle(
	local *LocalLearning, market *learningMarket, index int, marketAt time.Time, horizon time.Duration,
) error {

	if horizon <= 0 || len(lane.trace) == 0 {
		return nil
	}

	// Decisions issued before the first measured interval bind to that first
	// available measurement; subsequent changes cannot rewrite their horizon.
	for index := range lane.trace {
		if lane.trace[index].horizon <= 0 {
			lane.trace[index].horizon = horizon
		}
	}
	due := 0

	for due < len(lane.trace) && market.at.Sub(lane.trace[due].at) >= lane.trace[due].horizon {
		due++
	}

	if due == 0 {
		return nil
	}

	if err := lane.resolve(local, market, index, marketAt, lane.trace[:due], false); err != nil {
		return err
	}

	lane.trace = append(lane.trace[:0], lane.trace[due:]...)
	return nil
}

/*
resolve assigns each supplied decision its realized forward return-to-go.
Truncated marks decisions settled before their window closed because the
account ran out of capital: the outcome is real, the window is short, and the
journal says so rather than presenting it as a completed measurement.
*/
func (lane *learningLane) resolve(
	local *LocalLearning, market *learningMarket, index int, marketAt time.Time, due []learningExperience, truncated bool,
) error {
	capital := local.initial.Float64()

	for _, experience := range due {
		elapsed := market.at.Sub(experience.at).Seconds()

		if elapsed <= 0 {
			return errnie.Err(errnie.Validation, "local reward: positive issue-to-resolution time required", nil)
		}
		target := ((lane.equity-experience.value)/elapsed - experience.rate) / capital
		skillTarget := (lane.equity - experience.value) / capital
		prior, err := local.Knowledge.Resolve(experience, target)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[agent] failed to resolve action",
				err,
			))
		}

		/*
			Competence is measured over disjoint forward windows. A decision
			only enters the estimate when its window began at or after the end
			of the last admitted one, so no two accepted observations share
			tape across any market. Truncated windows never enter it at all:
			their return covers less forward tape than the horizon and is not
			comparable to one that ran its course. The other decisions still
			train their own action priors — this gate governs the competence
			estimate that grants execution authority, not what the agent learns.

			The model learns advantage (differential return); the skill meter
			measures absolute economic profitability (skillTarget).
		*/

		if lane.paper && local.execution.Skill != nil && !truncated &&
			!experience.at.Before(local.execution.Skill.window) {

			local.execution.Skill.window = market.at
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
		event.AbsoluteSkillTarget, event.BaselineRate = skillTarget, experience.rate
		event.Scope, event.GlobalPrior, event.SymbolPrior = experience.reading.Scope, experience.reading.Global, experience.reading.Symbol
		event.Authority = experience.authority
		event.Action, event.Power, event.Reduce = string(experience.action.Kind), experience.action.Power, experience.action.Reduce
		event.TargetUnit = "return_per_second"
		event.Target, event.Prior, event.Profit = target, prior, lane.outcome.TotalReward
		event.Horizon, event.Authorized, event.Truncated = experience.horizon, local.execution.Mode().String(), truncated
		market.events = append(market.events, event)
	}

	return nil
}

/*
recycle restarts a lane that can no longer act. An exploration wallet that has
spent its capital on execution costs is not evidence of anything further: it
is flat, cannot afford one venue lot, and every later decision it appears to
make is a forced wait. Its outstanding decisions resolve against the equity it
actually ended with — the truncated window is the outcome — and it starts a
new episode on a fresh clone of the same known capital.

Episodes are separate accounts in sequence. Their results are never summed
into a purported fundable balance; the retained total is a record of what this
lane realized, not capital anyone holds.
*/
func (lane *learningLane) recycle(
	local *LocalLearning, market *learningMarket, index int, book *spotbook.Book, marketAt time.Time,
) error {

	if lane.wallet.quantity.Sign() != 0 || lane.pending != 0 {
		lane.exhausted = false
		return nil
	}

	if lane.wallet.maximum(book, true).Sign() != 0 {
		lane.exhausted = false
		return nil
	}

	lane.exhausted = true

	if err := lane.resolve(local, market, index, marketAt, lane.trace, true); err != nil {
		return err
	}

	lane.trace = lane.trace[:0]
	lane.realized += lane.equity - local.initial.Float64()
	spent, _ := lane.wallet.restart(local.initial).Float64()
	lane.spent += spent
	lane.episodes++
	lane.ledger = AccountReward{}
	lane.outcome = learning.RewardOutcome{}
	lane.action = LearningAction{}

	event := lane.event(market, index, "recycled", lane.episodes, marketAt)
	event.Authorized, event.Horizon = local.execution.Mode().String(), market.horizon()
	market.events = append(market.events, event)
	return nil
}

/*
issue conditions the next decision on the impulse and the account's own
exposure. The previous action is deliberately absent from that identity: with
it, every decision changed the context it would be recalled under, so priors
never accumulated a second observation and exploration could never end.
*/
func (lane *learningLane) issue(local *LocalLearning, market *learningMarket, index int, book *spotbook.Book, marketAt time.Time) error {
	market.context = lane.wallet.context(market.sequence, book, lane.equity, market.context)
	market.actions = lane.wallet.actions(book, market.actions)

	// The policy lane reads the exploration lanes' evidence: that is the whole
	// point of exploring. It must also record its own outcomes under the same
	// identity, or its experience is written where nothing ever reads it.
	action, reading, err := local.Knowledge.Select(market.symbol, market.context, market.actions, !lane.paper)
	prior := reading.Selected
	selectedAction := action

	if err != nil {
		return errnie.Error(errnie.Err(
			errnie.Internal,
			"[agent] failed to select action",
			err,
		))
	}

	influence, authority, strength := 1.0, 0.0, 0.0

	for _, region := range market.regions {
		authority += region.Strength * region.Authority
		strength += region.Strength
	}

	if strength > 0 {
		authority /= strength
	}

	if lane.paper {
		influence = prior.Authority

		if action.Kind != types.ActionHold && !action.Reduce && prior.Mean <= 0 {
			action = LearningAction{Kind: types.ActionHold}
		}
	}

	requested := lane.wallet.request(book, action, influence, &lane.ladder)
	price := book.Asks.Low.Price

	if action.Reduce {
		price = book.Bids.High.Price
	}

	if action.Kind != types.ActionHold && (requested.Cmp(&lane.wallet.minimum) < 0 || new(big.Rat).Mul(requested, price.Rat()).Cmp(&lane.wallet.costMinimum) < 0) {
		action = LearningAction{Kind: types.ActionHold}
		requested = new(big.Rat)
	}

	if action != selectedAction {
		reading = local.Knowledge.Reading(market.symbol, market.context, action)
		prior = reading.Selected
	}

	identity, err := local.Knowledge.Issue(market.symbol, market.context, action, authority)

	if err != nil {
		return err
	}

	lane.pending, lane.action, lane.requested = identity, action, requested
	experience := learningExperience{
		id: identity, action: action, at: market.at,
		value: lane.equity, rate: lane.outcome.Rate, authority: authority,
	}
	experience.tokens = append([]uint64(nil), market.sequence...)
	experience.count = len(experience.tokens)
	experience.context = append([]uint64(nil), market.context...)
	experience.horizon = market.horizon()
	experience.reading = reading
	lane.trace = append(lane.trace, experience)
	lane.issued++
	local.decisions++
	event := lane.event(market, index, "issued", identity, marketAt)
	event.Context = append([]uint64(nil), market.context...)
	event.Scope, event.GlobalPrior, event.SymbolPrior = reading.Scope, reading.Global, reading.Symbol
	event.BaselineRate = lane.outcome.Rate
	for _, token := range market.sequence {
		event.Quantities = append(event.Quantities, local.Grid.Columns[token-1])
	}
	event.GridVersion, event.Authority, event.Quantity, event.Prior = market.gridVersion, authority, requested.FloatString(lane.wallet.pair.QtyPrecision), prior
	event.Horizon, event.Authorized = market.horizon(), local.execution.Mode().String()
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
		Cash:      lane.wallet.cash.FloatString(lane.wallet.scale),
		Inventory: lane.wallet.quantity.FloatString(lane.wallet.pair.QtyPrecision),
		Profit:    lane.outcome.TotalReward,
		Episode:   lane.episodes,
		Complete:  lane.complete,
		ValuedAt:  lane.outcome.Through.At,
	}
}
