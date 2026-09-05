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
	Model       *RewardModel
	Candidates  *CandidateBook
	Virtual     *VirtualPortfolio
	Exploration *AccountTeacher
	Actual      *AccountTeacher
	LastChoice  CapitalAction         `json:"lastChoice"`
	LastPrior   learning.PriorReading `json:"lastPrior"`
	Decisions   uint64                `json:"decisions"`
}

/* NewCapitalLearner creates one shared exploration wallet, never one wallet per candidate. */
func NewCapitalLearner(local *LocalLearning) *CapitalLearner {
	model := learning.NewModel[string, CapitalAction](knowledgeMemory)
	return &CapitalLearner{Model: model, History: CapitalHistory{knowledge: local.Knowledge, model: model}, Candidates: NewCandidateBook(local.recordCandidate),
		Virtual: NewVirtualPortfolio(local.initial), Exploration: NewAccountTeacher(model, "capital_virtual", local.Record),
		Actual: NewAccountTeacher(model, "capital_account", local.Record)}
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

		if err := capital.allocate(local, capital.Exploration, candidates, market.horizon(), true); err != nil {
			return err
		}
	}
	account, ok := local.execution.Desk.(ExecutionAccount)

	if !ok {
		return nil
	}
	actual := account.Account()

	if !actual.Complete || actual.Mark.Version == 0 {
		capital.Actual.State = actual
		return nil
	}

	if err := capital.Actual.Observe(actual); err != nil {
		return err
	}

	if capital.Actual.pending != nil {
		return nil
	}
	candidates, err := capital.Candidates.Viable(market.at)

	if err != nil {
		return err
	}
	return capital.allocate(local, capital.Actual, candidates, market.horizon(), false)
}

/*
allocate compares WAIT and all currently fundable claims using learned outcomes.
Sorting supplies reproducible tie ordering only; positive evidence is never
ranked by arrival time, a manual symbol tier or an authored portfolio score.
*/
func (capital *CapitalLearner) allocate(local *LocalLearning, teacher *AccountTeacher, candidates []*EntryCandidate, horizon time.Duration, explore bool) error {
	if horizon <= 0 {
		return nil
	}
	state := teacher.State
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
	selected, prior, err := capital.Model.Select("capital", contexts[actions[0]], actions, explore,
		func(key string, _ []uint64, action CapitalAction) learning.PriorReading {
			return capital.Model.Recall(key, contexts[action], action)
		})

	if err != nil {
		return err
	}
	candidate := claims[selected]
	candidateID, authority := "", 1.0

	if candidate != nil {
		candidateID, horizon, authority = candidate.Record.ID, candidate.Record.Horizon, candidate.Record.Authority
	}
	alternatives := make([]hindsight.AllocationAlternative, 0, len(actions))
	for _, action := range actions {
		alternative := hindsight.AllocationAlternative{Symbol: action.Symbol, Action: string(action.Kind), Power: action.Power, Context: contexts[action], Prior: capital.Model.Recall("capital", contexts[action], action)}

		if claim := claims[action]; claim != nil {
			alternative.CandidateID = claim.Record.ID
		}
		alternatives = append(alternatives, alternative)
	}
	identity, err := teacher.Issue(selected, contexts[selected], candidateID, horizon, authority, local.now(), alternatives, local.Grid.Columns)

	if err != nil {
		return err
	}

	if explore {
		if candidate != nil && !capital.Virtual.Allocate(candidate) {
			panic("capital: selected virtual allocation lost its serialized reservation")
		}
		return nil
	}
	capital.LastChoice, capital.LastPrior = selected, prior
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
