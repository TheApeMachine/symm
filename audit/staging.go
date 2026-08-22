package audit

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/types"
)

/*
StagedDecision pairs an evaluated decision with its retention deadline.
*/
type StagedDecision struct {
	Decision *types.Decision
	Deadline time.Time
}

/*
Stager buffers decisions in memory until their forecast horizon elapses,
allowing hindsight or the regulator to resolve their outcome before writing
them to permanent audit storage. Useless decisions can be pruned without
ever hitting the disk.
*/
type Stager struct {
	recorder *Recorder
	mu       sync.Mutex
	buffer   map[string]StagedDecision
}

/*
NewStager creates a memory buffer for evaluated decisions.
*/
func NewStager(recorder *Recorder) *Stager {
	return &Stager{
		recorder: recorder,
		buffer:   make(map[string]StagedDecision),
	}
}

/*
Stage retains a decision until its outcome can be verified.
The horizon is expected to be provided; if not, a default is used.
*/
func (s *Stager) Stage(decision *types.Decision, horizon time.Duration) {
	if s == nil || decision == nil {
		return
	}
	
	s.mu.Lock()
	s.buffer[decision.ID] = StagedDecision{
		Decision: decision,
		Deadline: time.Now().Add(horizon),
	}
	s.mu.Unlock()
}

/*
Prune permanently deletes a decision from the staging buffer without recording it.
*/
func (s *Stager) Prune(decisionID string) {
	if s == nil {
		return
	}
	
	s.mu.Lock()
	delete(s.buffer, decisionID)
	s.mu.Unlock()
}

/*
Flush removes a decision from the buffer and writes it to the permanent recorder.
*/
func (s *Stager) Flush(decisionID string) error {
	if s == nil || s.recorder == nil {
		return nil
	}
	
	s.mu.Lock()
	staged, ok := s.buffer[decisionID]
	if ok {
		delete(s.buffer, decisionID)
	}
	s.mu.Unlock()
	
	if !ok {
		return nil
	}
	
	return Record(s.recorder, staged.Decision)
}

/*
Matured returns a list of decisions whose deadline has passed, meaning they
are ready for outcome evaluation.
*/
func (s *Stager) Matured() []*types.Decision {
	if s == nil {
		return nil
	}
	
	now := time.Now()
	var matured []*types.Decision
	
	s.mu.Lock()
	for _, staged := range s.buffer {
		if now.After(staged.Deadline) {
			matured = append(matured, staged.Decision)
		}
	}
	s.mu.Unlock()
	
	return matured
}
