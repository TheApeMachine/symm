package economics

import (
	"sync"
	"time"
)

/*
Desk coordinates labeling, forward-return resolution, and playbook gating.
*/
type Desk struct {
	ledger  *Ledger
	pending *Pending
	mu      sync.Mutex
	labels  []Label
}

/*
NewDesk instantiates execution economics for the trader desk.
*/
func NewDesk() *Desk {
	return &Desk{
		ledger:  NewLedger(),
		pending: NewPending(),
	}
}

/*
AllowsEntry delegates to the playbook ledger.
*/
func (desk *Desk) AllowsEntry(playbook string) bool {
	return desk.ledger.AllowsEntry(playbook)
}

/*
RecordEntry stores the entry label and begins forward tracking.
*/
func (desk *Desk) RecordEntry(label Label) {
	desk.append(label)
	desk.pending.TrackEntry(
		label.Symbol, label.Playbook, label.FillPrice, label.RoundTripCostPct, label.At,
	)
}

/*
RecordExit stores the exit label and updates the ledger.
*/
func (desk *Desk) RecordExit(label Label) {
	desk.append(label)
	desk.pending.Drop(label.Symbol)
	desk.ledger.RecordNet(label.Playbook, label.NetReturn)
}

/*
ResolveForward matures pending forward labels for one symbol.
*/
func (desk *Desk) ResolveForward(symbol string, lastPrice float64, now time.Time) []Label {
	forwardLabels := desk.pending.ResolveForward(symbol, lastPrice, now)

	for _, label := range forwardLabels {
		desk.append(label)
		desk.ledger.RecordNet(label.Playbook, label.NetReturn)
	}

	return forwardLabels
}

/*
RecentLabels returns a copy of recorded labels (tests and audit).
*/
func (desk *Desk) RecentLabels() []Label {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	out := make([]Label, len(desk.labels))
	copy(out, desk.labels)

	return out
}

/*
PlaybookStats returns ledger stats for one playbook.
*/
func (desk *Desk) PlaybookStats(playbook string) (count int, mean float64) {
	return desk.ledger.Stats(playbook)
}

func (desk *Desk) append(label Label) {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	desk.labels = append(desk.labels, label)

	if len(desk.labels) > maxPlaybookSamples {
		desk.labels = desk.labels[len(desk.labels)-maxPlaybookSamples:]
	}
}
