package strategy

import (
	"math"
	"math/big"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/* AccountState is an immutable authoritative mark and its observable allocation inputs. */
type AccountState struct {
	Mark       EquityMark        `json:"mark"`
	ActualCash string            `json:"actualCash"`
	Committed  string            `json:"committed"`
	Cash       string            `json:"cash"`
	Positions  map[string]string `json:"positions"`
	Complete   bool              `json:"complete"`
	Reason     string            `json:"reason,omitempty"`
}

/* Context appends measured free-capital and held-instrument facts to Region context. */
func (state AccountState) Context(regions []uint64) []uint64 {
	context := append([]uint64(nil), regions...)
	context = append(context, 0)
	cash, valid := new(big.Rat).SetString(state.Cash)

	if !valid {
		panic("account: invalid authoritative cash")
	}
	amount, _ := cash.Float64()
	fraction := 0.0

	if state.Mark.Equity > 0 {
		fraction = amount / state.Mark.Equity
	}
	band := uint64(0)

	if fraction > 0 {
		band = uint64(max(0, -math.Floor(math.Log2(fraction)))) + 1
	}
	context = append(context, band)
	symbols := make([]string, 0, len(state.Positions))
	for symbol := range state.Positions {
		symbols = append(symbols, symbol)
	}
	slices.Sort(symbols)
	for _, symbol := range symbols {
		// UTF-8 bytes provide stable, reversible instrument identities without a
		// mutable numeric registry or a collision-prone hash. Zero is a delimiter.
		for _, character := range []byte(symbol) {
			context = append(context, uint64(character)+1)
		}
		context = append(context, 0)
	}
	return context
}

/* CapitalAction names a current allocation or retaining cash. */
type CapitalAction struct {
	Symbol string       `json:"symbol"`
	Kind   types.Action `json:"kind"`
	Power  uint16       `json:"power"`
}

/* allocationExperience freezes the account inputs and baseline before later marks resolve it. */
type allocationExperience struct {
	ID          string
	ticket      uint64
	action      CapitalAction
	context     []uint64
	state       AccountState
	rate        float64
	horizon     time.Duration
	candidateID string
	authority   float64
	at          time.Time
}

/* AccountTeacher assigns delayed elapsed-time wallet reward only to capital decisions. */
type AccountTeacher struct {
	Model           *RewardModel
	ledger          AccountReward
	pending         *allocationExperience
	State           AccountState           `json:"state"`
	Outcome         learning.RewardOutcome `json:"outcome"`
	Target          float64                `json:"target"`
	Resolved        uint64                 `json:"resolved"`
	MFE             float64                `json:"mfe"`
	MAE             float64                `json:"mae"`
	TimeToPositive  time.Duration          `json:"timeToPositiveNs"`
	TimeToBreakeven time.Duration          `json:"timeToBreakevenNs"`
	Holding         time.Duration          `json:"holdingNs"`
	Trajectory      []EquityMark           `json:"trajectory"`
	mode            string
	record          func(hindsight.LearningEvent) error
}

/* NewAccountTeacher keeps allocation evidence separate from local action priors and Skill. */
func NewAccountTeacher(model *RewardModel, mode string, record func(hindsight.LearningEvent) error) *AccountTeacher {
	return &AccountTeacher{Model: model, mode: mode, record: record}
}

/* Issue freezes a decision before its account can supply any future outcomes. */
func (teacher *AccountTeacher) Issue(action CapitalAction, context []uint64, candidateID string, horizon time.Duration, authority float64, at time.Time, alternatives []hindsight.AllocationAlternative, quantities [][2]string) (string, error) {
	if teacher.pending != nil || !teacher.State.Complete || horizon <= 0 || at.Before(teacher.State.Mark.At) {
		return "", errnie.Err(errnie.Validation, "account teacher: complete idle account and measured horizon required", nil)
	}
	names := make([][2]string, 0)
	for _, token := range context {
		if token == 0 {
			break
		}

		if token > uint64(len(quantities)) {
			return "", errnie.Err(errnie.Validation, "account teacher: Region quantity identity missing", nil)
		}
		names = append(names, quantities[token-1])
	}
	ticket, err := teacher.Model.Issue("capital", context, action, authority)

	if err != nil {
		return "", err
	}
	state := teacher.State
	state.Positions = make(map[string]string, len(teacher.State.Positions))
	for symbol, quantity := range teacher.State.Positions {
		state.Positions[symbol] = quantity
	}
	teacher.pending = &allocationExperience{ID: uuid.NewString(), ticket: ticket, action: action,
		context: append([]uint64(nil), context...), state: state, rate: teacher.Outcome.Rate,
		horizon: horizon, candidateID: candidateID, authority: authority, at: at}
	teacher.MFE, teacher.MAE, teacher.TimeToPositive, teacher.TimeToBreakeven, teacher.Holding = 0, 0, 0, 0, 0
	event := hindsight.LearningEvent{ID: ticket, Symbol: "account", Mode: teacher.mode, Kind: "portfolio_issued",
		At: at, CapitalSymbol: action.Symbol, Alternatives: alternatives, TargetUnit: "return_per_second", PortfolioID: teacher.pending.ID, CandidateID: candidateID, Context: teacher.pending.context,
		Action: string(action.Kind), Power: action.Power, Authority: authority, BaselineRate: teacher.pending.rate,
		Cash: state.Cash, Horizon: horizon, Quantities: names, Account: &state.Mark, AccountPositions: state.Positions}
	return teacher.pending.ID, teacher.record(event)
}

/*
Observe measures continuous account equity and resolves disjoint allocation windows.
The capital target is normalized differential growth per elapsed second. Thus
identical growth over unequal durations is distinguishable even from a zero
initial baseline rate. Unknown funding never becomes a fabricated zero.
*/
func (teacher *AccountTeacher) Observe(state AccountState) error {
	if !state.Complete {
		teacher.State = state
		return nil
	}

	if state.Mark.Version == teacher.State.Mark.Version && state.Mark == teacher.State.Mark {
		teacher.State = state
		return nil
	}
	outcome, err := teacher.ledger.Measure(state.Mark)

	if err != nil {
		return err
	}
	teacher.State, teacher.Outcome = state, outcome

	if teacher.mode == "capital_account" {
		identity := ""

		if teacher.pending != nil {
			identity = teacher.pending.ID
		}

		if err := teacher.record(hindsight.LearningEvent{ID: state.Mark.Version, Symbol: "account", Mode: teacher.mode, Kind: "portfolio_mark", At: state.Mark.At, PortfolioID: identity, Account: &state.Mark, Profit: outcome.TotalReward}); err != nil {
			return err
		}
	}
	teacher.Trajectory = append(teacher.Trajectory, state.Mark)

	if len(teacher.Trajectory) > recentReviewed {
		teacher.Trajectory = teacher.Trajectory[len(teacher.Trajectory)-recentReviewed:]
	}
	experience := teacher.pending

	if experience == nil {
		return nil
	}
	elapsed := state.Mark.At.Sub(experience.at)
	growth := (state.Mark.Equity - experience.state.Mark.Equity) - (state.Mark.NetFunding - experience.state.Mark.NetFunding)
	teacher.MFE, teacher.MAE, teacher.Holding = max(teacher.MFE, growth), min(teacher.MAE, growth), elapsed

	if growth > 0 && teacher.TimeToPositive == 0 {
		teacher.TimeToPositive = elapsed
	}

	if growth >= 0 && teacher.MAE < 0 && teacher.TimeToBreakeven == 0 {
		teacher.TimeToBreakeven = elapsed
	}

	if elapsed < experience.horizon {
		return nil
	}
	capital := teacher.ledger.initial.Equity

	if capital <= 0 || elapsed <= 0 {
		return errnie.Err(errnie.Validation, "account teacher: positive initial equity and elapsed interval required", nil)
	}
	teacher.Target = (growth/elapsed.Seconds() - experience.rate) / capital
	prior, err := teacher.Model.Resolve(experience.ticket, teacher.Target)

	if err != nil {
		return err
	}
	teacher.Resolved++
	teacher.pending = nil
	return teacher.record(hindsight.LearningEvent{ID: experience.ticket, Symbol: "account", Mode: teacher.mode,
		Kind: "portfolio_resolved", At: state.Mark.At, PortfolioID: experience.ID, CandidateID: experience.candidateID,
		Target: teacher.Target, TargetUnit: "return_per_second", Profit: growth, Prior: prior, Horizon: elapsed, Account: &state.Mark})
}
