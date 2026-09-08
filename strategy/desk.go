package strategy

import (
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/krakenfx/api-go/v2/pkg/decimal"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/system"
)

/*
A desk of independent traders sharing one memory.

Several traders meet the same market at the same instant with their own capital,
and they do not agree: each reaches for a different part of what is feasible, so
the same development is tried several ways at once and the tape gets to settle
which reading was right.

They learn into one model, not several. There will only ever be one agent when
this trades an account, so what is being built here is that single agent's
memory — the traders are how it is explored, not what it becomes. Experience
enters the shared model weighted by how well the tape agreed with it, which is
what makes the memory the consolidation of what worked rather than an average
of everything anyone tried.
*/
type Desk struct {
	Traders   []*Trader
	Knowledge *Knowledge

	/*
		Recent keeps the last settled verdicts for inspection. The model has
		already taken what it needs from these; they are retained so an operator
		can see what the tape said and why.
	*/
	Recent []Verdict

	Settled  uint64 `json:"settled"`
	Agreed   uint64 `json:"agreed"`
	Disputed uint64 `json:"disputed"`

	price   *broker.Price
	initial *decimal.Decimal
	actions []LearningAction
	rotate  uint64
}

/* recentVerdicts bounds what the desk retains purely for inspection. */
const recentVerdicts = 60

/* NewDesk starts every trader on its own copy of the same known capital. */
func NewDesk(knowledge *Knowledge, price *broker.Price, initial *decimal.Decimal) *Desk {
	desk := &Desk{Knowledge: knowledge, price: price, initial: initial.Copy()}

	for id := range system.Cfg.Learning.Traders {
		desk.Traders = append(desk.Traders, NewTrader(id, initial, price))
	}

	return desk
}

/*
Observe wakes every trader on one instrument that has just lit up, and lets each
decide what to do about it.

The development is the ordered recent history of this instrument's measured
state — now, then what came before it. Every trader sees the same one; what
differs is the wallet each is holding it against and how far each is willing to
reach beyond what the memory already prefers.
*/
func (desk *Desk) Observe(
	symbol string,
	book *spotbook.Book,
	development []uint64,
	at time.Time,
	sequence hindsight.CaptureSequence,
) ([]TraderDecision, error) {
	if len(development) == 0 || book == nil {
		return nil, nil
	}
	decisions := make([]TraderDecision, 0, len(desk.Traders))
	desk.rotate++

	for _, trader := range desk.Traders {
		feasible, err := trader.Feasible(symbol, book, desk.actions)

		if err != nil {
			return nil, err
		}
		desk.actions = feasible

		if len(feasible) == 0 {
			continue
		}
		action := desk.choose(trader, symbol, feasible, development)
		decision, made, err := trader.Execute(symbol, action, book, development, at, sequence)

		if err != nil {
			return nil, err
		}

		if made {
			decisions = append(decisions, decision)
		}
	}

	return decisions, nil
}

/*
choose picks one trader's move.

The first trader takes what the shared memory prefers, so the desk always
contains one reading of what has been learned so far. The others are spread
across the rest of what is feasible and rotate with each observation, so every
move a trader could make is tried by somebody rather than only the ones the
memory already likes. A memory that only ever sees its own preference confirmed
learns nothing about the alternatives it is passing over.
*/
func (desk *Desk) choose(
	trader *Trader, symbol string, feasible []LearningAction, development []uint64,
) LearningAction {
	state := trader.State(symbol)

	if trader.ID == 0 {
		action, _, err := desk.Knowledge.Select(symbol, state, development, feasible, false)

		if err == nil {
			return action
		}
	}
	reach := (trader.ID + int(desk.rotate)) % len(feasible)

	return feasible[reach]
}

/*
Settle asks the tape about every decision it can now answer, folds what it says
into the shared memory, and retires those decisions.

A decision is only settled once a move covering it has finished developing. Until
then it stays open: judging it against a move still in progress would be judging
the trader against a guess, which is the one thing the whole arrangement exists
to avoid.
*/
func (desk *Desk) Settle(
	episodes []hindsight.Episode,
) error {
	if len(episodes) == 0 {
		return nil
	}
	bySymbol := map[string][]hindsight.Episode{}
	for _, episode := range episodes {
		if episode.Confirmed {
			bySymbol[episode.Symbol] = append(bySymbol[episode.Symbol], episode)
		}
	}

	for _, trader := range desk.Traders {
		open := trader.Open[:0]

		for _, decision := range trader.Open {
			verdict, ready, err := Grade(decision, bySymbol[decision.Symbol], trader.Wealth)
			if err != nil {
				return err
			}

			if !ready {
				open = append(open, decision)
				continue
			}

			if err := desk.consolidate(verdict); err != nil {
				return err
			}
			trader.Graded++
			trader.Observed++
			trader.Quality += (verdict.Grade - trader.Quality) / trader.Observed
			desk.record(verdict)
		}
		trader.Open = open
	}

	return nil
}

/*
consolidate writes one settled verdict into the shared memory.

What is trained is the development the trader was looking at, against the move
it chose there. A verdict the tape agreed with reinforces that pairing; one it
disagreed with trains the same pairing negatively, which is what stops the
memory from repeating it. Both are evidence — a move that was wrong is as
informative as one that was right, and discarding the failures would leave a
memory that only knows what it already does.
*/
func (desk *Desk) consolidate(verdict Verdict) error {
	decision := verdict.Decision
	// Tape geometry has its own evidence scope. It cannot be averaged into
	// the economic-return scopes populated by actual wallet measurements.
	if err := desk.Knowledge.Model.Observe(
		[2]string{decision.Symbol, "tape:" + decision.State}, decision.Development, decision.Action,
		verdict.Tape, verdict.Clock, 1, [2]string{"", "tape:" + decision.State},
	); err != nil {
		return err
	}
	desk.Settled++

	if verdict.Tape > 0 {
		desk.Agreed++
	}

	if verdict.Tape < 0 {
		desk.Disputed++
	}
	return nil
}

/* record retains a verdict for inspection without unbounded growth. */
func (desk *Desk) record(verdict Verdict) {
	desk.Recent = append(desk.Recent, verdict)

	if len(desk.Recent) > recentVerdicts {
		desk.Recent = append(desk.Recent[:0], desk.Recent[len(desk.Recent)-recentVerdicts:]...)
	}
}

/* Mark values every trader's wallet against the current executable books. */
func (desk *Desk) Mark(books LearningBook) {
	for _, trader := range desk.Traders {
		trader.Mark(books)
	}
}

/*
Best names the trader whose moves the tape has agreed with most.

It is reported rather than promoted. All of them write into the same memory, so
the desk has no leader — this only says whose reading has been closest, which is
what an operator watching several disagreeing wallets actually wants to know.
*/
func (desk *Desk) Best() *Trader {
	var best *Trader

	for _, trader := range desk.Traders {
		if trader.Observed == 0 {
			continue
		}

		if best == nil || trader.Quality > best.Quality {
			best = trader
		}
	}

	return best
}

/* Open counts decisions across the desk that the tape has not yet settled. */
func (desk *Desk) Open() int {
	open := 0

	for _, trader := range desk.Traders {
		open += len(trader.Open)
	}

	return open
}

/* Holding counts positions currently carried across every trader's wallet. */
func (desk *Desk) Holding() int {
	held := 0

	for _, trader := range desk.Traders {
		for _, position := range trader.Positions {
			if position.quantity.Sign() > 0 {
				held++
			}
		}
	}

	return held
}
