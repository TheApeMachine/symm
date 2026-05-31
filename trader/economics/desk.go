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
PerformanceSummary aggregates closed trade economics for desk reporting.
ClosedTrades counts realized exits; ProfitableTrades and LosingTrades partition
those by sign of NetReturn; PositiveNetReturn and NegativeNetReturn sum the
winning and losing legs; MeanProfitHoldMS averages hold time on winners only.
*/
type PerformanceSummary struct {
	ClosedTrades      int     `json:"closed_trades"`
	ProfitableTrades  int     `json:"profitable_trades"`
	LosingTrades      int     `json:"losing_trades"`
	PositiveNetReturn float64 `json:"positive_net_return"`
	NegativeNetReturn float64 `json:"negative_net_return"`
	MeanProfitHoldMS  float64 `json:"mean_profit_hold_ms"`
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
PerformanceSummary returns closed-trade economics aggregated from desk labels.
*/
func (desk *Desk) PerformanceSummary() PerformanceSummary {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	summary := PerformanceSummary{}
	profitHoldMS := int64(0)

	for _, label := range desk.labels {
		if label.Event != "exit" {
			continue
		}

		summary.ClosedTrades++

		if label.NetReturn > 0 {
			summary.ProfitableTrades++
			summary.PositiveNetReturn += label.NetReturn
			profitHoldMS += label.HeldMS

			continue
		}

		if label.NetReturn < 0 {
			summary.LosingTrades++
			summary.NegativeNetReturn += label.NetReturn
		}
	}

	if summary.ProfitableTrades > 0 {
		summary.MeanProfitHoldMS = float64(profitHoldMS) / float64(summary.ProfitableTrades)
	}

	return summary
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
