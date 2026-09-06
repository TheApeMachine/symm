package strategy

import (
	"math/big"
	"slices"
	"strings"
	"time"

	spotbook "github.com/krakenfx/api-go/v2/pkg/book"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/* ExecutionAccount supplies authoritative account observations without venue calls. */
type ExecutionAccount interface{ Account() AccountState }

/* CapitalLearner learns allocation in a finite wallet and listens to the actual account teacher. */
type CapitalLearner struct {
	History     CapitalHistory
	Knowledge   *CapitalKnowledge
	Candidates  *CandidateBook
	Virtual     *VirtualPortfolio
	Exploration *AccountTeacher
	Actual      *AccountTeacher
	LastChoice  CapitalAction  `json:"lastChoice"`
	LastReading CapitalReading `json:"lastReading"`
	Decisions   uint64         `json:"decisions"`
}

/* NewCapitalLearner creates one shared exploration wallet, never one wallet per candidate. */
func NewCapitalLearner(local *LocalLearning) *CapitalLearner {
	knowledge := NewCapitalKnowledge()
	return &CapitalLearner{Knowledge: knowledge, History: CapitalHistory{knowledge: local.Knowledge, capital: knowledge}, Candidates: NewCandidateBook(local.recordCandidate),
		Virtual: NewVirtualPortfolio(local.initial), Exploration: NewAccountTeacher(knowledge, "capital_virtual", local.Record),
		Actual: NewAccountTeacher(knowledge, "capital_account", local.Record)}
}

/* Step settles finite-wallet observations before issuing new allocation decisions. */
func (capital *CapitalLearner) Step(local *LocalLearning, symbol string) error {
	market := local.markets[symbol]

	if market == nil || len(market.lanes) == 0 {
		return nil
	}
	var err error
	local.books.Book(symbol, func(book *spotbook.Book) {
		if book == nil || book.Bids == nil || book.Asks == nil || book.Bids.High == nil || book.Asks.Low == nil || book.Bids.High.Price.Cmp(book.Asks.Low.Price) >= 0 {
			return
		}
		err = capital.Virtual.Step(local, market, book)
	})

	if err != nil {
		return err
	}
	state := capital.Virtual.Snapshot(market.at)

	if err := capital.Exploration.Observe(state); err != nil {
		return err
	}

	if capital.Exploration.pending == nil && capital.Virtual.pending == nil && state.Complete {
		candidates, err := capital.Candidates.Viable(market.at)

		if err != nil {
			return err
		}

		if err := capital.allocate(local, capital.Exploration, candidates, true); err != nil {
			return err
		}
	}
	account, ok := local.execution.Desk.(ExecutionAccount)

	if !ok {
		return nil
	}
	actual := account.Account()

	if err := capital.Actual.Observe(actual); err != nil {
		return err
	}

	if capital.Actual.pending != nil || !actual.Complete || actual.Mark.Version == 0 {
		return nil
	}
	candidates, err := capital.Candidates.Viable(market.at)

	if err != nil {
		return err
	}
	return capital.allocate(local, capital.Actual, candidates, false)
}

/*
allocate compares WAIT and all currently fundable claims using learned outcomes.
Sorting supplies reproducible tie ordering only; positive evidence is never
ranked by arrival time, a manual symbol tier or an authored portfolio score.
*/
func (capital *CapitalLearner) allocate(local *LocalLearning, teacher *AccountTeacher, candidates []*EntryCandidate, explore bool) error {
	state := teacher.State
	at := local.now()
	cash, valid := new(big.Rat).SetString(state.Cash)

	if !valid {
		panic("capital: malformed authoritative cash")
	}
	slices.SortFunc(candidates, func(left, right *EntryCandidate) int { return strings.Compare(left.Record.Symbol, right.Record.Symbol) })
	actions := []CapitalAction{{Kind: types.ActionHold}}
	contexts := map[CapitalAction][]uint64{actions[0]: state.Context(nil)}
	claims := make(map[CapitalAction]*EntryCandidate)
	for _, candidate := range candidates {
		if !explore && local.execution.Mode() != ModeTrading {
			if err := capital.Candidates.Outcome(candidate, "authorization blocked", local.now(), "", "increase authority unavailable"); err != nil {
				return err
			}
			continue
		}

		if candidate.cost.Cmp(cash) > 0 {
			if !explore {
				if err := capital.Candidates.Outcome(candidate, "insufficient capital", local.now(), "", "fee-inclusive cost exceeds available cash"); err != nil {
					return err
				}
			}
			continue
		}
		action := CapitalAction{Symbol: candidate.Record.Symbol, Kind: candidate.action.Kind, Power: candidate.action.Power}
		actions = append(actions, action)
		contexts[action] = append(append([]uint64(nil), candidate.Record.Context...), contexts[actions[0]]...)
		claims[action] = candidate
	}
	horizon, horizonSource := capital.horizon(local, teacher, candidates, at)

	if horizon <= 0 {
		return nil
	}
	selected, _, err := capital.Knowledge.Model.Select([2]string{teacher.mode, ""}, contexts[actions[0]], actions, explore,
		func(_ [2]string, _ []uint64, action CapitalAction) learning.PriorReading {
			return capital.Knowledge.Reading(contexts[action], action).Selected
		})

	if err != nil {
		return err
	}
	candidate := claims[selected]
	candidateID, authority := "", 1.0

	if candidate != nil {
		candidateID, horizon, authority = candidate.Record.ID, candidate.Record.Horizon, candidate.Record.Authority
		horizonSource = "selected candidate horizon"
	}
	alternatives := make([]hindsight.AllocationAlternative, 0, len(actions))
	for _, action := range actions {
		reading := capital.Knowledge.Reading(contexts[action], action)
		alternative := hindsight.AllocationAlternative{Symbol: action.Symbol, Action: string(action.Kind), Power: action.Power,
			Context: contexts[action], Prior: reading.Selected, Source: reading.Source, Scope: reading.Scope,
			Virtual: reading.Virtual.Selected, Actual: reading.Actual.Selected}

		if claim := claims[action]; claim != nil {
			alternative.CandidateID = claim.Record.ID
		}
		alternatives = append(alternatives, alternative)
	}
	reading := capital.Knowledge.Reading(contexts[selected], selected)
	identity, err := teacher.Issue(selected, contexts[selected], candidateID, horizon, authority, at, alternatives, local.Grid.Columns, horizonSource)

	if err != nil {
		return err
	}

	if explore {
		if candidate != nil && !capital.Virtual.Allocate(candidate, teacher.pending.receipt) {
			panic("capital: selected virtual allocation lost its serialized reservation")
		}
		return nil
	}
	capital.LastChoice, capital.LastReading = selected, reading
	capital.Decisions++
	for _, alternative := range candidates {
		if alternative == candidate {
			continue
		}
		state := "lost learned competition"

		if candidate == nil {
			state = "wait chosen"
		}

		if alternative.cost.Cmp(cash) > 0 || local.execution.Mode() != ModeTrading {
			continue
		}

		if err := capital.Candidates.Outcome(alternative, state, local.now(), identity, ""); err != nil {
			return err
		}
	}

	if candidate == nil {
		return nil
	}
	candidate.selected = true
	intent := candidate.Intent
	intent.PortfolioID, intent.Mode, intent.Skill = identity, local.execution.Mode(), local.execution.Skill.Reading()
	intent.Allocation = teacher.pending.receipt
	// The frozen local action describes its independent wallet. Both local
	// ENTER and SCALE UP are buy claims; the actual lot chooses broker mechanics.
	intent.Kind = types.ActionEnter

	if _, held := state.Positions[candidate.Record.Symbol]; held {
		intent.Kind = types.ActionScale
	}

	if err := capital.Candidates.Outcome(candidate, "selected", local.now(), identity, ""); err != nil {
		return err
	}

	if err := local.flush(); err != nil {
		return err
	}
	return local.execution.Submit(intent)
}

/*
horizon ends WAIT at the first known candidate expiry or held-exposure review.
Without either, the shared account's measured observation interval defines its
next evaluation. The triggering symbol contributes no special clock.
*/
func (capital *CapitalLearner) horizon(local *LocalLearning, teacher *AccountTeacher, candidates []*EntryCandidate, at time.Time) (time.Duration, string) {
	var horizon time.Duration
	source := "account observation interval"
	for _, candidate := range candidates {
		remaining := candidate.Record.At.Add(candidate.Record.Horizon).Sub(at)

		if candidate.Current(at) && remaining > 0 && (horizon == 0 || remaining < horizon) {
			horizon, source = remaining, "earliest viable candidate expiry"
		}
	}
	for symbol := range teacher.State.Positions {
		market := local.markets[symbol]

		if market == nil {
			continue
		}
		measured := market.horizon()

		if measured > 0 && (horizon == 0 || measured < horizon) {
			horizon, source = measured, "held exposure horizon"
		}
	}

	if horizon == 0 {
		horizon = teacher.Outcome.Elapsed
	}

	return horizon, source
}
