package hindsight

import (
	"github.com/theapemachine/symm/nomagique/learning"
	"time"
)

/*
LearningEvent records a prospectively issued virtual decision, its execution,
or its completed return-to-go. At is the local producer's actual decision or
valuation time. MarketAt is the triggering market message time; a resident
book may already be newer when read. These are separate facts.

Truncated marks a return-to-go assigned before its measurement window closed,
because the account it was measured against ran out of capital to act with.
Its outcome is real but its window is shorter than the horizon, and it must
not be read as a completed measurement.

Episode counts the account restarts this lane has been through, so a record
is never read as belonging to a continuous balance. Horizon is the forward
measurement window in force when the record was made, and Authorized is the
execution authority the agent held: a decision issued under one authority
cannot be reinterpreted later under another.
*/
type LearningEvent struct {
	Allocation       *AllocationResult       `json:"allocation,omitempty"`
	HorizonSource    string                  `json:"horizonSource,omitempty"`
	TargetUnit       string                  `json:"targetUnit,omitempty"`
	CapitalSymbol    string                  `json:"capitalSymbol,omitempty"`
	Alternatives     []AllocationAlternative `json:"alternatives,omitempty"`
	Account          *AccountMark            `json:"account,omitempty"`
	AccountPositions map[string]string       `json:"accountPositions,omitempty"`

	Candidate       *CandidateRecord `json:"candidate,omitempty"`
	CandidateResult *CandidateResult `json:"candidateResult,omitempty"`

	Run                 RunID                 `json:"run,omitempty"`
	Capture             CaptureIdentity       `json:"capture"`
	Quantities          [][2]string           `json:"quantities,omitempty"`
	Scope               string                `json:"scope,omitempty"`
	GlobalPrior         learning.PriorReading `json:"globalPrior"`
	SymbolPrior         learning.PriorReading `json:"symbolPrior"`
	AbsoluteSkillTarget float64               `json:"absoluteSkillTarget"`
	BaselineRate        float64               `json:"baselineRate"`
	CandidateID         string                `json:"candidateId,omitempty"`
	PortfolioID         string                `json:"portfolioId,omitempty"`

	ID          uint64                `json:"id"`
	Symbol      string                `json:"symbol"`
	Lane        int                   `json:"lane"`
	Mode        string                `json:"mode"`
	Kind        string                `json:"kind"`
	At          time.Time             `json:"at"`
	MarketAt    time.Time             `json:"marketAt"`
	GridVersion uint64                `json:"gridVersion"`
	Context     []uint64              `json:"context,omitempty"`
	Action      string                `json:"action"`
	Power       uint16                `json:"power"`
	Reduce      bool                  `json:"reduce"`
	Quantity    string                `json:"quantity,omitempty"`
	Gross       string                `json:"gross,omitempty"`
	Fee         string                `json:"fee,omitempty"`
	Cash        string                `json:"cash"`
	Inventory   string                `json:"inventory"`
	Authority   float64               `json:"authority"`
	Target      float64               `json:"target,omitempty"`
	Profit      float64               `json:"profit"`
	Episode     uint64                `json:"episode"`
	Truncated   bool                  `json:"truncated,omitempty"`
	Horizon     time.Duration         `json:"horizonNs"`
	Authorized  string                `json:"authorized,omitempty"`
	Complete    bool                  `json:"complete"`
	ValuedAt    time.Time             `json:"valuedAt"`
	Prior       learning.PriorReading `json:"prior"`
}
