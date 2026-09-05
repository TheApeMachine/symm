package strategy

import (
	"math/big"
	"sync/atomic"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
)

/* EntryCandidate owns the live validity of one frozen prospective claim on capital. */
type EntryCandidate struct {
	Record         hindsight.CandidateRecord
	Intent         ExecutionIntent
	action         LearningAction
	quantity, cost *big.Rat
	bid            *big.Rat
	ladder         depthLadder
	valid          atomic.Bool
	selected       bool
	State          string
}

/* Current can be checked by the venue worker without reading workspace-owned maps. */
func (candidate *EntryCandidate) Current(at time.Time) bool {
	return candidate != nil && candidate.valid.Load() && !at.Before(candidate.Record.At) && at.Sub(candidate.Record.At) < candidate.Record.Horizon
}

/* CandidateBook owns current claims by symbol and a bounded operator history. */
type CandidateBook struct {
	current map[string]*EntryCandidate
	recent  []hindsight.CandidateResult
	viable  []*EntryCandidate
	record  func(hindsight.LearningEvent) error
}

/* NewCandidateBook connects ephemeral state to the existing durable journal. */
func NewCandidateBook(record func(hindsight.LearningEvent) error) *CandidateBook {
	return &CandidateBook{current: make(map[string]*EntryCandidate), record: record}
}

/* Publish freezes an already priced candidate before any capital decision sees it. */
func (book *CandidateBook) Publish(candidate *EntryCandidate) error {
	if candidate.Record.ID == "" || candidate.Record.Horizon <= 0 || candidate.quantity.Sign() <= 0 || candidate.cost.Sign() <= 0 {
		return errnie.Err(errnie.Validation, "candidate: identity, measured horizon and positive executable economics required", nil)
	}

	if err := book.Invalidate(candidate.Record.Symbol, candidate.Record.At, "context changed"); err != nil {
		return err
	}
	candidate.valid.Store(true)
	candidate.State = "current"
	book.current[candidate.Record.Symbol] = candidate
	return book.record(hindsight.LearningEvent{ID: candidate.Record.Decision, Symbol: candidate.Record.Symbol,
		Mode: "candidate", Kind: "candidate", At: candidate.Record.At, CandidateID: candidate.Record.ID, Candidate: &candidate.Record})
}

/* Invalidate revokes even a queued selection when its originating market changes. */
func (book *CandidateBook) Invalidate(symbol string, at time.Time, detail string) error {
	candidate := book.current[symbol]

	if candidate == nil {
		return nil
	}
	candidate.valid.Store(false)
	delete(book.current, symbol)
	return book.Outcome(candidate, "stale", at, "", detail)
}

/* Outcome appends a distinct decision fact; the candidate's issue input stays frozen. */
func (book *CandidateBook) Outcome(candidate *EntryCandidate, state string, at time.Time, portfolio, detail string) error {
	if candidate.State == state {
		return nil
	}

	if state == "authorization blocked" || state == "insufficient capital" || state == "no longer executable" || state == "repricing failed" {
		candidate.valid.Store(false)

		if book.current[candidate.Record.Symbol] == candidate {
			delete(book.current, candidate.Record.Symbol)
		}
	}

	candidate.State = state
	result := hindsight.CandidateResult{ID: candidate.Record.ID, State: state, At: at, PortfolioID: portfolio, Detail: detail}
	book.recent = append(book.recent, result)

	if len(book.recent) > recentReviewed {
		book.recent = book.recent[len(book.recent)-recentReviewed:]
	}
	return book.record(hindsight.LearningEvent{ID: candidate.Record.Decision, Symbol: candidate.Record.Symbol,
		Mode: "candidate", Kind: "candidate_status", At: at, CandidateID: candidate.Record.ID, CandidateResult: &result})
}

/* Viable visits only currently present claims, retiring expired decisions visibly. */
func (book *CandidateBook) Viable(at time.Time) ([]*EntryCandidate, error) {
	candidates := book.viable[:0]
	for symbol, candidate := range book.current {
		if !candidate.Current(at) {
			if err := book.Invalidate(symbol, at, "measured horizon elapsed"); err != nil {
				return nil, err
			}
			continue
		}

		if !candidate.selected {
			candidates = append(candidates, candidate)
		}
	}
	book.viable = candidates
	return candidates, nil
}
