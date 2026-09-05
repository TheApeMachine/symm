package strategy

import (
	"time"

	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
)

/* CapitalView is an operator copy of prospective competition and delayed account feedback. */
type CapitalView struct {
	Choice      CapitalAction               `json:"choice"`
	Prior       learning.PriorReading       `json:"prior"`
	Decisions   uint64                      `json:"decisions"`
	Actual      AccountLearningView         `json:"actual"`
	Exploration AccountLearningView         `json:"exploration"`
	Candidates  []CandidateView             `json:"candidates"`
	Outcomes    []hindsight.CandidateResult `json:"outcomes"`
	Demand      string                      `json:"demand"`
}

/* CandidateView joins a frozen record with its current observable validity. */
type CandidateView struct {
	hindsight.CandidateRecord
	State   string        `json:"state"`
	Current bool          `json:"current"`
	Age     time.Duration `json:"ageNs"`
}

/* AccountLearningView exposes account economics without exporting mutable learner state. */
type AccountLearningView struct {
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
	Pending         string                 `json:"pending"`
}
