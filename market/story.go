package market

import (
	"sync"

	"github.com/theapemachine/symm/market/perspectives"
)

/*
EntryVerdictRecord stores one playbook flat-entry outcome for dashboards.
*/
type EntryVerdictRecord struct {
	Name   string
	Action perspectives.ActionType
	Regime perspectives.Regime
	Why    string
}

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	mu            sync.RWMutex
	entries       map[string][]Decision
	entryVerdicts map[string][]EntryVerdictRecord
	exits         map[string][]Decision
	activeNames   map[string][]string
}

func NewStory() *Story {
	return &Story{
		entries:       make(map[string][]Decision),
		entryVerdicts: make(map[string][]EntryVerdictRecord),
		exits:         make(map[string][]Decision),
		activeNames:   make(map[string][]string),
	}
}

/*
RecordEntry stores the latest flat-entry verdicts for one symbol.
*/
func (story *Story) RecordEntry(symbol string, decisions []Decision) {
	story.mu.Lock()
	defer story.mu.Unlock()

	story.entries[symbol] = decisions

	names := make([]string, 0, len(decisions))

	for _, verdict := range decisions {
		names = append(names, verdict.Name)
	}

	story.activeNames[symbol] = names
}

/*
RecordEntryVerdicts stores every flat-entry playbook outcome, including denies.
*/
func (story *Story) RecordEntryVerdicts(symbol string, verdicts []EntryVerdict) {
	story.mu.Lock()
	defer story.mu.Unlock()

	records := make([]EntryVerdictRecord, 0, len(verdicts))

	for _, verdict := range verdicts {
		records = append(records, EntryVerdictRecord{
			Name:   verdict.Name,
			Action: verdict.Action,
			Regime: verdict.Regime,
			Why:    entryVerdictWhy(verdict),
		})
	}

	story.entryVerdicts[symbol] = records
}

func entryVerdictWhy(verdict EntryVerdict) string {
	if verdict.Trace == nil {
		return perspectives.ActionLabel(verdict.Action)
	}

	step, ok := verdict.Trace.LastStep()

	if !ok {
		return perspectives.ActionLabel(verdict.Action)
	}

	if step.Category != perspectives.CategoryTypeNone {
		return step.Category.String() + "_" + perspectives.ActionLabel(step.Action)
	}

	return perspectives.ActionLabel(verdict.Action)
}

/*
LatestEntryVerdicts returns the last recorded playbook outcomes for a symbol.
*/
func (story *Story) LatestEntryVerdicts(symbol string) []EntryVerdictRecord {
	story.mu.RLock()
	defer story.mu.RUnlock()

	return story.entryVerdicts[symbol]
}

/*
RecordExit stores the latest exit verdicts considered for one symbol.
*/
func (story *Story) RecordExit(symbol string, decisions []Decision) {
	story.mu.Lock()
	defer story.mu.Unlock()

	story.exits[symbol] = decisions
}

/*
ActivePlaybooks returns the names of playbooks that last authorized entry.
*/
func (story *Story) ActivePlaybooks(symbol string) []string {
	story.mu.RLock()
	defer story.mu.RUnlock()

	return story.activeNames[symbol]
}

/*
LatestEntries returns the last recorded entry decisions for a symbol.
*/
func (story *Story) LatestEntries(symbol string) []Decision {
	story.mu.RLock()
	defer story.mu.RUnlock()

	return story.entries[symbol]
}
