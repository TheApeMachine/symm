package hindsight

import "github.com/theapemachine/symm/nomagique/learning"

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
}
