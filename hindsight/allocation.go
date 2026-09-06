package hindsight

import (
	"github.com/theapemachine/symm/nomagique/learning"
	"time"
)

/* AllocationResult records a selected allocation's actual execution state separately from its input. */
type AllocationResult struct {
	State  string    `json:"state"`
	At     time.Time `json:"at"`
	Detail string    `json:"detail,omitempty"`
}

/*
	AllocationAlternative is one prospectively available capital action and the

specific reading that participated in competition, including WAIT.
*/
type AllocationAlternative struct {
	Symbol      string                `json:"symbol"`
	Action      string                `json:"action"`
	Power       uint16                `json:"power"`
	CandidateID string                `json:"candidateId,omitempty"`
	Context     []uint64              `json:"context"`
	Prior       learning.PriorReading `json:"prior"`
	Source      string                `json:"source"`
	Scope       string                `json:"scope"`
	Virtual     learning.PriorReading `json:"virtual"`
	Actual      learning.PriorReading `json:"actual"`
}
