package economics

import (
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/theapemachine/symm/config"
)

type pendingReject struct {
	symbol        string
	playbook      string
	reason        string
	price         float64
	roundTripCost float64
	notionalEUR   float64
	rejectedAt    time.Time
}

type resolvedReject struct {
	pendingReject
	forwardReturn float64
	netReturn     float64
	missed        bool
}

/*
RegretBreakdown aggregates counterfactual gate-reject outcomes for one key.
*/
type RegretBreakdown struct {
	Key              string  `json:"key"`
	Count            int     `json:"count"`
	MissedProfitable int     `json:"missed_profitable"`
	MissedForwardEUR float64 `json:"missed_forward_eur"`
}

/*
RegretSummary is the headless eval counterfactual for blocked entries.
*/
type RegretSummary struct {
	GateRejectsTracked  int               `json:"gate_rejects_tracked"`
	GateRejectsResolved int               `json:"gate_rejects_resolved"`
	MissedProfitable    int               `json:"missed_profitable"`
	MissedForwardEUR    float64           `json:"missed_forward_eur"`
	ByPlaybook          []RegretBreakdown `json:"by_playbook"`
	ByReason            []RegretBreakdown `json:"by_reason"`
}

/*
RejectRegret tracks counterfactual forward returns for gate rejects.
*/
type RejectRegret struct {
	mu           sync.Mutex
	pending      []pendingReject
	resolved     []resolvedReject
	lastTracked  map[string]time.Time
	dedupeWindow time.Duration
}

/*
NewRejectRegret instantiates a gate-reject counterfactual tracker.
*/
func NewRejectRegret() *RejectRegret {
	return &RejectRegret{
		lastTracked:  make(map[string]time.Time),
		dedupeWindow: 60 * time.Second,
	}
}

/*
SetDedupeWindow configures how often identical rejects are sampled.
*/
func (regret *RejectRegret) SetDedupeWindow(window time.Duration) {
	if window <= 0 {
		return
	}

	regret.mu.Lock()
	regret.dedupeWindow = window
	regret.mu.Unlock()
}

/*
Track registers one gate reject for forward labeling.
*/
func (regret *RejectRegret) Track(
	symbol, playbook, reason string,
	price, roundTripCost, notionalEUR float64,
	at time.Time,
) {
	if symbol == "" || playbook == "" || reason == "" || price <= 0 || notionalEUR <= 0 {
		return
	}

	regret.mu.Lock()
	defer regret.mu.Unlock()

	key := rejectKey(symbol, playbook, reason)

	if last, ok := regret.lastTracked[key]; ok && at.Sub(last) < regret.dedupeWindow {
		return
	}

	regret.lastTracked[key] = at
	regret.pending = append(regret.pending, pendingReject{
		symbol:        symbol,
		playbook:      playbook,
		reason:        reason,
		price:         price,
		roundTripCost: roundTripCost,
		notionalEUR:   notionalEUR,
		rejectedAt:    at,
	})
}

/*
ResolveForward matures pending rejects for one symbol.
*/
func (regret *RejectRegret) ResolveForward(symbol string, lastPrice float64, now time.Time) {
	if lastPrice <= 0 {
		return
	}

	window := forwardWindow()

	regret.mu.Lock()
	defer regret.mu.Unlock()

	remaining := regret.pending[:0]

	for _, sample := range regret.pending {
		if sample.symbol != symbol {
			remaining = append(remaining, sample)

			continue
		}

		if now.Sub(sample.rejectedAt) < window {
			remaining = append(remaining, sample)

			continue
		}

		regret.resolveSample(sample, lastPrice)
	}

	regret.pending = remaining
}

/*
Flush resolves all pending rejects using the last known price per symbol.
*/
func (regret *RejectRegret) Flush(lastPrices map[string]float64) {
	regret.mu.Lock()
	defer regret.mu.Unlock()

	remaining := regret.pending[:0]

	for _, sample := range regret.pending {
		lastPrice, ok := lastPrices[sample.symbol]

		if !ok || lastPrice <= 0 {
			remaining = append(remaining, sample)

			continue
		}

		regret.resolveSample(sample, lastPrice)
	}

	regret.pending = remaining
}

/*
Summary returns aggregated counterfactual regret.
*/
func (regret *RejectRegret) Summary() RegretSummary {
	regret.mu.Lock()
	defer regret.mu.Unlock()

	playbookBuckets := make(map[string]*RegretBreakdown)
	reasonBuckets := make(map[string]*RegretBreakdown)

	summary := RegretSummary{
		GateRejectsTracked: len(regret.resolved) + len(regret.pending),
	}

	for _, item := range regret.resolved {
		summary.GateRejectsResolved++

		if item.missed {
			summary.MissedProfitable++
			summary.MissedForwardEUR += item.netReturn * item.notionalEUR
		}

		bumpBucket(playbookBuckets, item.playbook, item.missed, item.netReturn*item.notionalEUR)
		bumpBucket(reasonBuckets, item.reason, item.missed, item.netReturn*item.notionalEUR)
	}

	summary.ByPlaybook = sortedBreakdowns(playbookBuckets)
	summary.ByReason = sortedBreakdowns(reasonBuckets)

	return summary
}

func (regret *RejectRegret) resolveSample(sample pendingReject, lastPrice float64) {
	forward := 0.0

	if sample.price > 0 {
		forward = (lastPrice - sample.price) / sample.price
	}

	net := NetForwardReturn(forward, sample.roundTripCost)
	missed := net > 0

	regret.resolved = append(regret.resolved, resolvedReject{
		pendingReject: sample,
		forwardReturn: forward,
		netReturn:     net,
		missed:        missed,
	})
}

func rejectKey(symbol, playbook, reason string) string {
	return symbol + "|" + playbook + "|" + reason
}

func forwardWindow() time.Duration {
	window := config.System.ExecutionForwardWindow

	if window <= 0 {
		return 30 * time.Second
	}

	return window
}

func bumpBucket(
	buckets map[string]*RegretBreakdown,
	key string,
	missed bool,
	missedEUR float64,
) {
	bucket, ok := buckets[key]

	if !ok {
		bucket = &RegretBreakdown{Key: key}
		buckets[key] = bucket
	}

	bucket.Count++

	if missed {
		bucket.MissedProfitable++
		bucket.MissedForwardEUR += missedEUR
	}
}

func sortedBreakdowns(buckets map[string]*RegretBreakdown) []RegretBreakdown {
	out := make([]RegretBreakdown, 0, len(buckets))

	for _, bucket := range buckets {
		out = append(out, *bucket)
	}

	slices.SortFunc(out, func(left, right RegretBreakdown) int {
		return strings.Compare(left.Key, right.Key)
	})

	return out
}
