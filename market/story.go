package market

import "sync"

/*
Story holds the latest playbook verdicts per symbol for dashboards and audits.
*/
type Story struct {
	mu          sync.RWMutex
	entries     map[string][]Decision
	exits       map[string][]Decision
	activeNames map[string][]string
}

func NewStory() *Story {
	return &Story{
		entries:     make(map[string][]Decision),
		exits:       make(map[string][]Decision),
		activeNames: make(map[string][]string),
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
