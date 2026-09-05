package strategy

import (
	"math"
	"math/big"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/* learningExperience retains only an unresolved decision's return reference. */
type learningExperience struct {
	id          uint64
	action      LearningAction
	value, rate float64
	at          time.Time
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
	trace                   []learningExperience
	equity                  float64
	complete                bool
	issued, fills, resolved uint64
	lastPrior               learning.PriorReading
}

/* transition settles first, then chooses new actions at changed impulses. */
func (agent *Agent) transition(
	market *learningMarket,
	book *spotbook.Book,
	marketAt time.Time,
	changed bool,
) error {
	for index := range market.lanes {
		lane := &market.lanes[index]
		hadPending := lane.pending != 0

		if !changed && !hadPending && lane.version != 0 && lane.complete {
			continue
		}

		if hadPending {
			quantity, gross, fee := lane.wallet.fill(book, lane.action, lane.requested)
			event := lane.event(market, index, "filled", lane.pending, marketAt)
			event.Complete = false
			event.Quantity, event.Gross, event.Fee = quantity.FloatString(lane.wallet.pair.QtyPrecision), gross.FloatString(lane.wallet.scale), fee.FloatString(lane.wallet.scale)

			if lane.action.Kind == types.ActionHold {
				event.Kind = "waited"
			}

			if lane.action.Kind != types.ActionHold && quantity.Sign() == 0 {
				event.Kind = "rejected"
			}

			if quantity.Sign() > 0 {
				lane.fills++
			}

			market.events = append(market.events, event)
			lane.pending = 0
		}

		mark, complete := lane.wallet.mark(book)
		lane.complete = complete

		if !complete {
			market.status = "open inventory exceeds visible liquidation depth"
			continue
		}

		lane.equity, _ = mark.Float64()
		lane.version++

		outcome, err := lane.ledger.Measure(EquityMark{
			At: market.at, Version: lane.version, Equity: lane.equity, HasFunding: true,
		})

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[agent] failed to measure lane",
				err,
			))
		}

		lane.outcome = outcome

		if hadPending {
			event := &market.events[len(market.events)-1]
			event.Profit, event.Complete, event.ValuedAt = outcome.TotalReward, true, market.at
		}

		if lane.wallet.quantity.Sign() == 0 {
			if err := agent.complete(market, index, marketAt); err != nil {
				return errnie.Error(errnie.Err(
					errnie.Internal,
					"[agent] failed to complete lane",
					err,
				))
			}
		}

		if !changed && lane.issued != 0 {
			continue
		}

		if len(market.sequence) == 0 && lane.wallet.quantity.Sign() == 0 {
			continue
		}

		if err := agent.issue(market, index, book, marketAt); err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[agent] failed to issue action",
				err,
			))
		}
	}

	return nil
}

/*
complete assigns every unresolved action its own subsequent return-to-go.
The economic ledger counts each account change once. Target averages retain
correlated experiences, not claims that each action independently caused the
whole result. Elapsed-time cost uses only the rate known when the action issued.
*/
func (agent *Agent) complete(market *learningMarket, index int, marketAt time.Time) error {
	lane := &market.lanes[index]

	for _, experience := range lane.trace {
		target := (lane.equity - experience.value - experience.rate*market.at.Sub(experience.at).Seconds()) / agent.initial.Float64()
		prior, err := agent.Model.Resolve(experience.id, target)

		if err != nil {
			return errnie.Error(errnie.Err(
				errnie.Internal,
				"[agent] failed to resolve action",
				err,
			))
		}

		lane.lastPrior = prior
		lane.resolved++
		agent.resolved++

		event := lane.event(market, index, "resolved", experience.id, marketAt)
		event.Action, event.Power, event.Reduce = string(experience.action.Kind), experience.action.Power, experience.action.Reduce
		event.Target, event.Prior, event.Profit = target, prior, lane.outcome.TotalReward
		market.events = append(market.events, event)
	}

	lane.trace = lane.trace[:0]
	return nil
}

/* issue conditions the next decision on regions, inventory and previous action. */
func (agent *Agent) issue(market *learningMarket, index int, book *spotbook.Book, marketAt time.Time) error {
	lane := &market.lanes[index]
	market.context = append(market.context[:0], market.sequence...)
	exposure := uint64(0)

	if lane.wallet.quantity.Sign() > 0 && lane.equity > 0 {
		gross, _ := new(big.Rat).Mul(&lane.wallet.quantity, book.Bids.High.Price.Rat()).Float64()
		fraction := gross / lane.equity
		exposure = uint64(max(0, -math.Floor(math.Log2(fraction)))) + 1
	}

	previous := uint64(0)

	for position, kind := range []types.Action{
		types.ActionHold, types.ActionEnter, types.ActionExit, types.ActionScale,
	} {
		if lane.action.Kind == kind {
			previous = uint64(position)
		}
	}

	// Zero separates region IDs (which start at one) from numeric lane context.
	reducing := uint64(0)

	if lane.action.Reduce {
		reducing = 1
	}

	market.context = append(market.context, 0, exposure, previous, uint64(lane.action.Power), reducing)
	market.actions = lane.wallet.actions(book, market.actions)
	key := [2]string{market.symbol, "virtual"}
	action, prior, err := agent.Model.Select(key, market.context, market.actions, !lane.paper)

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
		key[1] = "paper"
		influence = prior.Authority

		if action.Kind != types.ActionHold && !action.Reduce && prior.Mean <= 0 {
			action = LearningAction{Kind: types.ActionHold}
		}
	}

	requested := lane.wallet.request(book, action, influence)
	price := book.Asks.Low.Price

	if action.Reduce {
		price = book.Bids.High.Price
	}

	if action.Kind != types.ActionHold && (requested.Cmp(&lane.wallet.minimum) < 0 || new(big.Rat).Mul(requested, price.Rat()).Cmp(&lane.wallet.costMinimum) < 0) {
		action = LearningAction{Kind: types.ActionHold}
		requested = new(big.Rat)
	}

	identity, err := agent.Model.Issue(key, market.context, action, authority)

	if err != nil {
		return err
	}

	lane.pending, lane.action, lane.requested = identity, action, requested
	lane.trace = append(lane.trace, learningExperience{id: identity, action: action, at: market.at, value: lane.equity, rate: lane.outcome.Rate})
	lane.issued++
	agent.decisions++
	event := lane.event(market, index, "issued", identity, marketAt)
	event.Context = append([]uint64(nil), market.context...)
	event.GridVersion, event.Authority, event.Quantity, event.Prior = agent.Grid.Version, authority, requested.FloatString(lane.wallet.pair.QtyPrecision), prior
	market.events = append(market.events, event)
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
		mode = "paper"
	}

	return hindsight.LearningEvent{
		ID:        identity,
		Symbol:    market.symbol,
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
		Complete:  lane.complete,
		ValuedAt:  lane.outcome.Through.At,
	}
}
