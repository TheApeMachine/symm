package types

import "github.com/theapemachine/qpool"

/*
ScanOptions bounds the offline parallel branch scan.
*/
type ScanOptions struct {
	Workers           int
	MaxThresholds     int
	BeamWidth         int
	CandidateLimit    int
	MaxReasoningSteps int
	Guard             GuardOptions
	Budget            SearchBudget
	Pool              *qpool.Q
}

/*
ScanStats reports how much of the bounded space was scored.
*/
type ScanStats struct {
	Candidates int
	Workers    int
}
