package strategy

import (
	"fmt"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/broker/position"
	"github.com/theapemachine/symm/hindsight/recording"
	"github.com/theapemachine/symm/kraken/websocket"
	"github.com/theapemachine/symm/nomagique/learning/associative/agent"
	"github.com/theapemachine/symm/nomagique/learning/associative/reward"
)

/*
	Action identifies a resource fraction, independently of the current price.

Power is a binary subdivision of available cash or held inventory.
*/
type Action struct {
	Kind   string
	Power  uint16
	Reduce bool
}

/*
These bound an explorer's capacity, not its opinion. Each one removes an option
the wallet physically cannot sustain; none of them scores a symbol, forecasts a
price, or vetoes an entry the account could actually fund.
*/
const (
	// FlatPositionContext is the structural context for an unheld, non-stale position.
	FlatPositionContext uint64 = 1 << 63

	// MinimumOrderValue is the venue's smallest accepted order in quote
	// currency. A wallet below it cannot act at all — no size it could name
	// would be admitted — so its agent sits issuing waits and stops
	// contributing experience. It bounds renewal, never entry.
	MinimumOrderValue = 5.0

	// MaxConcurrentPositions caps how many symbols one wallet may be long at
	// once. Without it an explorer sprays its whole balance across every symbol
	// that ever looked interesting, ends at zero cash holding a dozen unrelated
	// tokens, and can no longer take any action but waiting — it stops
	// exploring precisely when its evidence is worst. The cap is what forces
	// capital to be recycled rather than merely allocated.
	MaxConcurrentPositions = 4

	// StalePositionHorizon is how long a position may sit before holding it
	// stops being one of the feasible things to do. An entry is a bet that a
	// move is developing; if the move has not developed by now the bet has been
	// answered, and the capital belongs back in the wallet. This withdraws the
	// hold option — it never submits an order on the agent's behalf.
	StalePositionHorizon = 12 * time.Minute

	// MaximumHoldingTime is the renewal backstop, not a trading rule. Past the
	// stale horizon the agent is already being made to choose an exit, so
	// inventory only survives this long when no exit was ever executable.
	MaximumHoldingTime = time.Hour
)

/*
	Trader adapts an independent account and its normal position regulators to

associative learning. It is owned by Learner's serialized state transitions.
*/
type Trader struct {
	Balance                                       *broker.Balance
	Positions                                     map[string]*position.Regulator
	Execution                                     Execution
	Initial, Equity, Profit, Realized, Unrealized *decimal.Decimal
	Wealth, Carried                               float64
	At                                            time.Time
	Version, Fills                                uint64
	Status                                        string
	Alternatives                                  []Action
	Quantities                                    map[Action]*decimal.Decimal
	Evaluations                                   map[string]map[uint64]*Evaluation
	Opened                                        map[string]time.Time
	LastEvaluation                                *Evaluation
	Recorder                                      *recording.Session
	ID                                            int
	Episode                                       uint64
	Fees                                          *decimal.Decimal
	api                                           *websocket.API
}

func NewTrader(
	api *websocket.API,
	price *broker.Price,
	quote string,
	cash *decimal.Decimal,
) (*Trader, error) {
	balance, err := broker.NewFundedBalance(quote, cash)

	if err != nil {
		return nil, err
	}

	return &Trader{
		api:     api,
		Balance: balance,
		Execution: Execution{
			Price:   price,
			Balance: balance,
		},
		Initial:     cash,
		Equity:      cash,
		Realized:    decimal.NewFromInt64(0),
		Unrealized:  decimal.NewFromInt64(0),
		Fees:        decimal.NewFromInt64(0),
		Profit:      decimal.NewFromInt64(0),
		Positions:   make(map[string]*position.Regulator),
		Opened:      make(map[string]time.Time),
		Evaluations: make(map[string]map[uint64]*Evaluation),
	}, nil
}

/*
	Feasible filters only available resources, venue minima and executable depth.

Book absence permits waiting; it is reported rather than invented as a price.

Two capacity limits join the wallet's own: a new symbol is only enterable while
the wallet is under MaxConcurrentPositions, and a position past
StalePositionHorizon loses its hold and scale-up options so the agent must
choose among the moves that return capital. Neither invents a price, predicts
one, or submits an order — they narrow what is possible, and the agent still
decides. Nothing here consults spread or a round-trip crossing cost.
*/
func (trader *Trader) Feasible(symbol string) ([]Action, []uint64, error) {
	idle := Action{Kind: "wait"}
	held := decimal.NewFromInt64(0)

	if regulator := trader.Positions[symbol]; regulator != nil {
		held = regulator.Holding.Qty
	}
	stale := false

	if held.Sign() > 0 {
		idle.Kind, stale = "hold", trader.stale(symbol)
	}
	trader.Alternatives = []Action{idle}
	trader.Quantities = make(map[Action]*decimal.Decimal)
	trader.Status = "book unavailable"
	var bid, ask *decimal.Decimal
	trader.api.Book(symbol, func(book *book.Book) {
		if book != nil && book.BestBid() != nil && book.BestAsk() != nil {
			bid, ask = book.BestBid().Price, book.BestAsk().Price
		}
	})
	state := []uint64{FlatPositionContext}

	if held.Sign() > 0 {
		state[0]++
	}

	// Staleness is part of the state the decision is conditioned on, so the
	// agent can learn what a position that stopped developing is worth.
	if stale {
		state[0] += 2
	}

	if bid == nil || ask == nil || trader.Execution.Price.FeeIfAvailable(symbol) == nil {
		return trader.Alternatives, state, nil
	}
	trader.Status = "learning"

	if held.Sign() > 0 {
		if err := trader.sizes(symbol, Action{Kind: "exit", Reduce: true}, held, bid); err != nil {
			return nil, nil, err
		}

		// Holding is withdrawn only once an executable exit actually exists. A
		// stale position no venue will take back still has to be held, and
		// saying otherwise would leave the agent nothing it could do.
		if stale && len(trader.Alternatives) > 1 {
			trader.Status = "stale position"
			trader.Alternatives = trader.Alternatives[1:]
			return trader.Alternatives, state, nil
		}
	}

	if trader.Balance.Cash().Sign() <= 0 {
		return trader.Alternatives, state, nil
	}
	kind := "enter"

	if held.Sign() > 0 {
		kind = "scale"
	}

	// Adding to a symbol already held commits no new position, so it is bounded
	// by cash alone; opening a new one is bounded by the concurrency cap.
	if held.Sign() == 0 && trader.open() >= MaxConcurrentPositions {
		trader.Status = "position capacity"
		return trader.Alternatives, state, nil
	}
	quantity, err := trader.Execution.Price.Affordable(symbol, trader.Balance.Cash(), ask)

	if err != nil {
		return nil, nil, err
	}

	if err := trader.sizes(symbol, Action{Kind: kind}, quantity, ask); err != nil {
		return nil, nil, err
	}
	return trader.Alternatives, state, nil
}

/* open counts the symbols this wallet currently holds inventory in. */
func (trader *Trader) open() int {
	count := 0

	for _, regulator := range trader.Positions {
		if regulator.Holding.Qty.Sign() > 0 {
			count++
		}
	}

	return count
}

/*
stale reports whether the current position in a symbol has been held past
StalePositionHorizon. Opened is stamped by this trader when a position is
established and cleared when it goes flat, so a re-entry is measured from its
own fill rather than inheriting the age of the position that preceded it.
*/
func (trader *Trader) stale(symbol string) bool {
	opened, exists := trader.Opened[symbol]
	return exists && trader.At.Sub(opened) >= StalePositionHorizon
}

/*
	sizes enumerates the binary resource fractions down to the venue's minimum.

No fixed exposure cap or prediction threshold participates.
*/
func (trader *Trader) sizes(symbol string, action Action, quantity, unit *decimal.Decimal) error {
	for trader.Execution.Price.Tradable(symbol, quantity, unit) {
		usable := false

		if action.Reduce {
			surface, err := trader.Execution.Price.Surface(symbol, quantity, trader.At)
			usable = err == nil && surface.FullyExecutable

			if err != nil {
				trader.Status = err.Error()

				if !errnie.IsUnprocessableContent(err) {
					return err
				}
			}
		}

		if !action.Reduce {
			cost, err := trader.Execution.Price.EntryCost(symbol, quantity)
			usable = err == nil && cost.Total.Cmp(trader.Balance.Cash()) <= 0

			if err != nil {
				trader.Status = err.Error()

				if !errnie.IsUnprocessableContent(err) {
					return err
				}
			}
		}

		if usable {
			trader.Alternatives = append(trader.Alternatives, action)
			trader.Quantities[action] = quantity
		}
		action.Power++

		if action.Reduce {
			action.Kind = "scale"
		}
		next, err := trader.api.Normalizer().FormatSize(symbol, quantity.Div(decimal.NewFromInt64(2)))

		if err != nil {
			return errnie.Error(err)
		}

		if next.Cmp(quantity) >= 0 {
			break
		}
		quantity = next
	}
	return nil
}

/*
	Execute submits through the normal regulator and applies its actual fill.

The retained evaluation points to the original decision, never a second model.
*/
func (trader *Trader) Execute(decision *agent.Decision[Action]) error {
	evaluation := &Evaluation{Decision: decision, Trader: trader.ID, Initial: trader.Initial}
	if trader.Evaluations[decision.Label] == nil {
		trader.Evaluations[decision.Label] = make(map[uint64]*Evaluation)
	}
	trader.Evaluations[decision.Label][decision.ID] = evaluation
	regulator := trader.Positions[decision.Label]
	evaluation.Quantity = decimal.NewFromInt64(0)

	if regulator != nil {
		evaluation.Quantity = regulator.Holding.Qty
	}
	trader.api.Book(decision.Label, func(current *book.Book) {
		if current != nil && current.BestBid() != nil {
			evaluation.Reference = current.BestBid().Price
		}
	})

	// A decision taken from a single feasible action was not a choice, and the
	// outcome it collects is evidence about the market rather than about the
	// action. Learner releases these immediately after execution instead of training them.
	evaluation.Forced = len(trader.Alternatives) < 2

	if decision.Action.Kind == "wait" || decision.Action.Kind == "hold" {
		for _, action := range trader.Alternatives {
			if action.Kind == "enter" && evaluation.Opportunity == nil {
				evaluation.Opportunity = trader.Quantities[action]
			}
		}
		return trader.Recorder.WriteDecision(evaluation.Row(""))
	}

	if regulator == nil {
		regulator = position.NewRegulator(trader.api, trader.Execution.Price, decision.Label, nil)
		regulator.Orders = &trader.Execution
		trader.Positions[decision.Label] = regulator
	}
	side := broker.BUY

	if decision.Action.Reduce {
		side = broker.SELL
	}
	trader.Execution.At = decision.At

	if err := regulator.Submit(fmt.Sprint(decision.ID), side, trader.Quantities[decision.Action]); err != nil {
		return err
	}

	if err := regulator.Apply(trader.Execution.Fill); err != nil {
		return err
	}
	evaluation.Quantity = trader.Execution.Fill.CumQty
	evaluation.Cost = trader.Execution.Fill.CumCost
	evaluation.Fee = trader.Execution.Fill.FeeUsdEquiv
	trader.mark(decision.Label, regulator, decision.At)
	trader.Fills++
	trader.Fees = trader.Fees.Add(evaluation.Fee)
	return trader.Recorder.WriteDecision(evaluation.Row(""))
}

/*
mark tracks when the current position in a symbol was established. It is stamped
on the fill that opens one and cleared when the symbol goes flat, so holding age
belongs to the position actually held rather than to the symbol's whole history.
*/
func (trader *Trader) mark(symbol string, regulator *position.Regulator, at time.Time) {
	if regulator.Holding.Qty.Sign() == 0 {
		delete(trader.Opened, symbol)
		return
	}

	if _, exists := trader.Opened[symbol]; !exists {
		trader.Opened[symbol] = at
	}
}

/*
	Objective values the whole wallet with the existing price owner. The return

unit is a fraction of initial funding; the core owns elapsed-time feedback.
A missing held book leaves the mark absent and the last valuation unchanged.
*/
func (trader *Trader) Objective() (*reward.Mark, error) {
	// Each worker owns one regulator and result slot. Wallet state is committed
	// only after all workers finish, including when a book cannot be valued.
	marks := make([]struct {
		regulator *position.Regulator
		status    string
	}, len(trader.Positions))
	index := 0

	for _, regulator := range trader.Positions {
		marks[index].regulator = regulator
		index++
	}
	var group errgroup.Group

	for index := range marks {
		regulator := marks[index].regulator

		if regulator.Holding.Qty.Sign() == 0 {
			continue
		}
		group.Go(func() error {
			available := false
			trader.api.Book(regulator.Holding.Symbol, func(book *book.Book) {
				available = book != nil && book.BestBid() != nil && book.BestAsk() != nil
			})

			if !available {
				marks[index].status = "awaiting book: " + regulator.Holding.Symbol
				return nil
			}

			if err := regulator.Mark(trader.At); err != nil {
				// Missing, crossed or insufficient depth leaves valuation absent.
				// Unexpected failures still fail the whole measurement phase.
				if errnie.IsNotFound(err) || errnie.IsUnprocessableContent(err) || errnie.IsValidation(err) {
					marks[index].status = "unmarkable: " + regulator.Holding.Symbol
					return nil
				}
				return errnie.Error(err)
			}

			if regulator.Surface == nil {
				marks[index].status = "unmarkable: " + regulator.Holding.Symbol
			}
			return nil
		})
	}

	if err := group.Wait(); err != nil {
		return nil, errnie.Error(err)
	}
	equity := trader.Balance.Cash().SetScale(decimal.DefaultScale)
	realized := decimal.NewFromInt64(0)

	for _, mark := range marks {
		if mark.status != "" {
			trader.Status = mark.status
			return nil, nil
		}
		regulator := mark.regulator
		realized = realized.Add(regulator.Holding.RealizedPnL)

		if regulator.Holding.Qty.Sign() > 0 {
			equity = equity.Add(regulator.Surface.ExecutableValue)
		}
	}
	trader.Equity, trader.Profit = equity, equity.Sub(trader.Initial)
	trader.Realized, trader.Unrealized = realized, trader.Profit.Sub(realized)
	trader.Wealth = trader.Profit.Div(trader.Initial).Float64()

	trader.Status = "learning"

	// Carried holds the wealth of episodes already closed. Reporting it with
	// the current one keeps the objective continuous across a wallet renewal:
	// without it a bankrupt explorer refunded to par would read as a large
	// positive reward and train its last, losing decision as a success.
	return &reward.Mark{
		At: trader.At, Version: trader.Version, Value: trader.Carried + trader.Wealth,
	}, nil
}

/*
CheckAndReset renews an exhausted explorer wallet so the swarm keeps producing
experience. Exploration spends capital by design, and a drawn-down wallet whose
cash sits under the venue minimum has no feasible action left but waiting: it
generates no fills, no evaluations and no evidence. Renewal is episodic rather
than continuous — residual inventory is liquidated first, and a position still
inside MaximumHoldingTime defers the reset rather than being cut short, so the
episode that is being closed is graded on its own tape before the books reset.
Holding age comes from Opened, this trader's own record of when the current
position was established, because Holding.EntryAt survives a close and would
report the age of a position that no longer exists.

Realized history stays with the resolved evaluations already issued; only the
account is renewed. It reports whether an episode actually turned over.
*/
func (trader *Trader) CheckAndReset(initialFunding *decimal.Decimal) bool {
	if initialFunding == nil || initialFunding.Sign() <= 0 {
		return false
	}

	if trader.Balance.Cash().Cmp(decimal.NewFromFloat64(MinimumOrderValue)) >= 0 {
		return false
	}

	for symbol, regulator := range trader.Positions {
		if regulator.Holding.Qty.Sign() == 0 {
			continue
		}
		opened, tracked := trader.Opened[symbol]

		if tracked && trader.At.Sub(opened) < MaximumHoldingTime {
			return false
		}
	}
	trader.liquidate()

	// Value the closing episode after liquidation so its final proceeds and
	// fees are inside the wealth that gets carried forward. Inventory that
	// survived liquidation is by definition inventory no venue would price, so
	// an absent mark cannot be allowed to block the renewal it exists to
	// perform: the last measured wealth is carried instead.
	if _, err := trader.Objective(); err != nil {
		trader.Status = err.Error()
		return false
	}
	balance, err := broker.NewFundedBalance(trader.Balance.Quote, initialFunding)

	if err != nil {
		trader.Status = err.Error()
		return false
	}
	trader.Balance, trader.Execution.Balance = balance, balance
	trader.Positions = make(map[string]*position.Regulator)
	trader.Opened = make(map[string]time.Time)
	trader.Initial, trader.Equity = initialFunding, initialFunding
	trader.Realized, trader.Unrealized = decimal.NewFromInt64(0), decimal.NewFromInt64(0)
	trader.Profit, trader.Fees = decimal.NewFromInt64(0), decimal.NewFromInt64(0)
	trader.Carried, trader.Wealth = trader.Carried+trader.Wealth, 0
	trader.Episode++
	trader.Status = "replenished"
	return true
}

/*
liquidate sells whatever residual inventory the book will still take. A dust
holding no venue will fill is left unsold rather than assumed away; the renewed
account simply stops tracking it, because the position it describes belongs to
the episode being closed.
*/
func (trader *Trader) liquidate() {
	for symbol, regulator := range trader.Positions {
		if regulator.Holding.Qty.Sign() == 0 {
			continue
		}
		trader.Execution.At = trader.At

		if err := regulator.Exit(fmt.Sprintf("reset-%d-%d-%s", trader.ID, trader.Episode, symbol)); err != nil {
			continue
		}

		if err := regulator.Apply(trader.Execution.Fill); err != nil {
			continue
		}
		trader.Fills++
		trader.Fees = trader.Fees.Add(trader.Execution.Fill.FeeUsdEquiv)
	}
}
