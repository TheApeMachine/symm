package strategy

import (
	"sort"
	"sync"
)

/*
Evidence wraps one current compatibility snapshot while numerical producers
migrate to typed chronological epochs. It deliberately carries no synthetic
timestamp or unused value map because neither would preserve source meaning.
*/
type Evidence struct {
	Source   string
	Symbol   string
	Snapshot any
}

/*
EvidenceBook owns the current compatibility snapshot for every symbol and
source. The L3 analysis owner and the measurement/planning loop both access it,
so this migration-only book uses one narrow lock instead of pretending it has a
single writer. Typed epoch and decision journals remain lock-free and ordered.
*/
type EvidenceBook struct {
	mu      sync.RWMutex
	entries map[string]map[string]*Evidence
}

/*
NewEvidenceBook creates an empty current-evidence book.
*/
func NewEvidenceBook() *EvidenceBook {
	return &EvidenceBook{entries: map[string]map[string]*Evidence{}}
}

/*
Update replaces the current evidence for one symbol and source.
*/
func (book *EvidenceBook) Update(symbol string, evidence Evidence) {
	book.mu.Lock()
	defer book.mu.Unlock()

	if book.entries[symbol] == nil {
		book.entries[symbol] = map[string]*Evidence{}
	}

	book.entries[symbol][evidence.Source] = &evidence
}

/*
Symbols returns the journal's symbols in deterministic order.
*/
func (book *EvidenceBook) Symbols() []string {
	book.mu.RLock()
	defer book.mu.RUnlock()

	symbols := make([]string, 0, len(book.entries))

	for symbol := range book.entries {
		symbols = append(symbols, symbol)
	}

	sort.Strings(symbols)

	return symbols
}

/*
Values returns one symbol's current evidence in deterministic source order.
*/
func (book *EvidenceBook) Values(symbol string) ([]*Evidence, bool) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	entries, ok := book.entries[symbol]

	if !ok {
		return nil, false
	}

	sources := make([]string, 0, len(entries))

	for source := range entries {
		sources = append(sources, source)
	}

	sort.Strings(sources)
	values := make([]*Evidence, 0, len(sources))

	for _, source := range sources {
		values = append(values, entries[source])
	}

	return values, true
}

/*
Latest returns the newest evidence for a source.
*/
func (book *EvidenceBook) Latest(symbol string, source string) (any, bool) {
	book.mu.RLock()
	defer book.mu.RUnlock()

	entries, ok := book.entries[symbol]

	if !ok {
		return nil, false
	}

	evidence, ok := entries[source]

	if !ok {
		return nil, false
	}

	return evidence.Snapshot, true
}

/*
Delete removes one current source snapshot without disturbing the rest of the
symbol's evidence. Invalidating a model output must remove the stale output
rather than leave it eligible for a later strategy pass.
*/
func (book *EvidenceBook) Delete(symbol string, source string) {
	book.mu.Lock()
	defer book.mu.Unlock()

	entries, ok := book.entries[symbol]

	if !ok {
		return
	}

	delete(entries, source)

	if len(entries) == 0 {
		delete(book.entries, symbol)
	}
}

/*
NewEvidence wraps a snapshot in the Evidence type.
*/
func NewEvidence(source string, symbol string, snapshot any) *Evidence {
	return &Evidence{
		Source:   source,
		Symbol:   symbol,
		Snapshot: snapshot,
	}
}
