package economics

import (
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
)

/*
Desk coordinates labeling, forward-return resolution, and playbook gating.
*/
type Desk struct {
	ledger  *Ledger
	pending *Pending
	regret  *RejectRegret
	mu      sync.Mutex
	labels  []Label
}

/*
NewDesk instantiates execution economics for the trader desk.
*/
func NewDesk() *Desk {
	desk := &Desk{
		ledger:  NewLedger(),
		pending: NewPending(),
		regret:  NewRejectRegret(),
	}

	if cooldown := config.System.AuditGateRejectCooldown; cooldown > 0 {
		desk.regret.SetDedupeWindow(cooldown)
	}

	return desk
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
TrackGateReject registers a blocked entry for counterfactual forward labeling.
*/
func (desk *Desk) TrackGateReject(
	symbol, playbook, reason string,
	price, roundTripCost, notionalEUR float64,
	at time.Time,
) {
	desk.regret.Track(symbol, playbook, reason, price, roundTripCost, notionalEUR, at)
}

/*
ResolveGateReject matures pending gate rejects for one symbol.
*/
func (desk *Desk) ResolveGateReject(symbol string, lastPrice float64, now time.Time) {
	desk.regret.ResolveForward(symbol, lastPrice, now)
}

/*
FlushGateReject resolves all pending gate rejects at replay end.
*/
func (desk *Desk) FlushGateReject(lastPrices map[string]float64) {
	desk.regret.Flush(lastPrices)
}

/*
GateRegretSummary returns aggregated counterfactual gate-reject outcomes.
*/
func (desk *Desk) GateRegretSummary() RegretSummary {
	return desk.regret.Summary()
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
