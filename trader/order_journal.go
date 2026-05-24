package trader

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

/*
OrderJournal records live order lifecycle events for reconciliation.
*/
type OrderJournal struct {
	mu      sync.Mutex
	path    string
	entries []OrderJournalEntry
}

/*
OrderJournalEntry is one persisted broker event.
*/
type OrderJournalEntry struct {
	TS          time.Time `json:"ts"`
	Event       string    `json:"event"`
	Symbol      string    `json:"symbol"`
	Side        string    `json:"side"`
	OrderID     string    `json:"order_id"`
	StopOrderID string    `json:"stop_order_id,omitempty"`
	NotionalEUR float64   `json:"notional_eur,omitempty"`
	FillPrice   float64   `json:"fill_price,omitempty"`
	Reason      string    `json:"reason,omitempty"`
}

/*
NewOrderJournal creates an in-memory journal optionally backed by a log file.
*/
func NewOrderJournal(logDir string) *OrderJournal {
	path := ""

	if logDir != "" {
		path = filepath.Join(logDir, "orders.jsonl")
	}

	return &OrderJournal{path: path}
}

/*
RecordEntry appends one order event and persists when configured.
*/
func (journal *OrderJournal) RecordEntry(entry OrderJournalEntry) {
	if journal == nil {
		return
	}

	if entry.TS.IsZero() {
		entry.TS = time.Now().UTC()
	}

	journal.mu.Lock()
	journal.entries = append(journal.entries, entry)
	path := journal.path
	journal.mu.Unlock()

	if path == "" {
		return
	}

	payload, err := json.Marshal(entry)

	if err != nil {
		return
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)

	if err != nil {
		return
	}

	_, _ = file.Write(append(payload, '\n'))
	_ = file.Close()
}

/*
Entries returns a copy of recorded journal rows.
*/
func (journal *OrderJournal) Entries() []OrderJournalEntry {
	if journal == nil {
		return nil
	}

	journal.mu.Lock()
	defer journal.mu.Unlock()

	copied := make([]OrderJournalEntry, len(journal.entries))
	copy(copied, journal.entries)

	return copied
}

/*
LatestStopOrderID returns the most recent stop order id for one symbol.
*/
func (journal *OrderJournal) LatestStopOrderID(symbol string) string {
	if journal == nil || symbol == "" {
		return ""
	}

	journal.mu.Lock()
	defer journal.mu.Unlock()

	for index := len(journal.entries) - 1; index >= 0; index-- {
		entry := journal.entries[index]

		if entry.Symbol != symbol {
			continue
		}

		if entry.StopOrderID != "" {
			return entry.StopOrderID
		}
	}

	return ""
}

/*
LoadFromDisk replays persisted journal rows into memory.
*/
func (journal *OrderJournal) LoadFromDisk() {
	if journal == nil || journal.path == "" {
		return
	}

	file, err := os.Open(journal.path)

	if err != nil {
		return
	}

	defer file.Close()

	journal.mu.Lock()
	defer journal.mu.Unlock()

	journal.entries = journal.entries[:0]

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Bytes()

		if len(line) == 0 {
			continue
		}

		var entry OrderJournalEntry

		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}

		journal.entries = append(journal.entries, entry)
	}
}
