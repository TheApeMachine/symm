package broker

import (
	"math"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
)

/*
Match is the trie match behind a learned decision: the live path, how many
stored paths continue it, the actions those paths end in, and the path length
against the minimum confidence that admitted the action.
*/
type Match struct {
	At         time.Time      `json:"at"`
	Action     string         `json:"action"`
	Path       string         `json:"path"`
	Tokens     int            `json:"tokens"`
	Candidates int            `json:"candidates"`
	Actions    map[string]int `json:"actions"`
	Confidence int            `json:"confidence"`
	Threshold  int            `json:"threshold"`
	Span       time.Duration  `json:"span_ns"`
}

/*
Event kinds of a position's life.
*/
const (
	EventEntryMatch  = "entry_match"
	EventSizing      = "sizing"
	EventChildOrder  = "child_order"
	EventOrder       = "order_submitted"
	EventOrderFailed = "order_failed"
	EventFill        = "fill"
	EventRiskSell    = "risk_sell"
	EventExitMatch   = "exit_match"
	EventExit        = "exit"
	EventPlanEnded   = "plan_ended"
	EventClosed      = "closed"
	EventAbandoned   = "abandoned"
)

/*
LifeEvent is one moment of a position's life. At is the Desk's wall clock when
it was recorded; VenueAt is the venue time of the book version or fill it was
decided on, when there is one. Fields carry the numbers the decision used; a
map is written once and never changed after it is appended.
*/
type LifeEvent struct {
	At      time.Time      `json:"at"`
	VenueAt *time.Time     `json:"venue_at,omitempty"`
	Kind    string         `json:"kind"`
	Detail  string         `json:"detail"`
	Fields  map[string]any `json:"fields,omitempty"`
}

/*
Outcome is the result of a closed position: venue and shadow P&L with their
costs and fees, and the hold from the first fill to the close against the
expected hold it was sized for.
*/
type Outcome struct {
	VenueCost      float64       `json:"venue_cost"`
	VenueProceeds  float64       `json:"venue_proceeds"`
	VenueFees      float64       `json:"venue_fees"`
	VenueRealized  float64       `json:"venue_realized"`
	ShadowDefined  bool          `json:"shadow_defined"`
	ShadowCost     float64       `json:"shadow_cost"`
	ShadowProceeds float64       `json:"shadow_proceeds"`
	ShadowFees     float64       `json:"shadow_fees"`
	ShadowRealized float64       `json:"shadow_realized"`
	ShadowShort    float64       `json:"shadow_short"`
	ShadowUnpriced int           `json:"shadow_unpriced"`
	Hold           time.Duration `json:"hold_ns"`
	ExpectedHold   time.Duration `json:"expected_hold_ns"`
	Triggers       []string      `json:"triggers"`
}

/*
Lifecycle is the append-only event timeline of one position, from the match
that opened it to its outcome. The Desk retains every lifecycle for the
session.
*/
type Lifecycle struct {
	ID       string      `json:"id"`
	Symbol   string      `json:"symbol"`
	Status   string      `json:"status"`
	OpenedAt time.Time   `json:"opened_at"`
	ClosedAt *time.Time  `json:"closed_at,omitempty"`
	Events   []LifeEvent `json:"events"`
	Outcome  *Outcome    `json:"outcome,omitempty"`
}

func newLifecycle(symbol string) *Lifecycle {
	return &Lifecycle{
		ID:       uuid.NewString(),
		Symbol:   symbol,
		Status:   "entering",
		OpenedAt: time.Now().UTC(),
		Events:   make([]LifeEvent, 0),
	}
}

/*
record appends one event. venueAt is optional (zero for none). The caller
holds the Desk lock.
*/
func (life *Lifecycle) record(kind, detail string, venueAt time.Time, fields map[string]any) {
	if life == nil {
		return
	}

	// JSON has no infinities or NaN; a non-finite number is reported by
	// name so one undefined ratio never breaks the whole report.
	for key, value := range fields {
		if number, ok := value.(float64); ok && (math.IsInf(number, 0) || math.IsNaN(number)) {
			fields[key] = strconv.FormatFloat(number, 'g', -1, 64)
		}
	}

	event := LifeEvent{At: time.Now().UTC(), Kind: kind, Detail: detail, Fields: fields}
	if !venueAt.IsZero() {
		at := venueAt.UTC()
		event.VenueAt = &at
	}

	life.Events = append(life.Events, event)
}

/*
end closes the lifecycle with status. The caller holds the Desk lock.
*/
func (life *Lifecycle) end(status string) {
	if life == nil || life.ClosedAt != nil {
		return
	}

	at := time.Now().UTC()
	life.Status, life.ClosedAt = status, &at
}

/*
copy is a snapshot safe to hand out: events are append-only and their field
maps are never changed after appending, so sharing them is safe.
*/
func (life *Lifecycle) copy() Lifecycle {
	out := *life
	out.Events = slices.Clone(life.Events)
	return out
}

/*
Performance is measured over the closed lifecycles of the session: win rate
and mean return per trade on the shadow ledger (the evaluation truth) and on
the venue, beside their total P&L. A rate or mean is nil until it has a trade
to be measured over.
*/
type Performance struct {
	Closed           int      `json:"closed"`
	ShadowTrades     int      `json:"shadow_trades"`
	ShadowWins       int      `json:"shadow_wins"`
	ShadowWinRate    *float64 `json:"shadow_win_rate"`
	ShadowMeanReturn *float64 `json:"shadow_mean_return"`
	ShadowPnl        float64  `json:"shadow_pnl"`
	VenueWins        int      `json:"venue_wins"`
	VenueWinRate     *float64 `json:"venue_win_rate"`
	VenueMeanReturn  *float64 `json:"venue_mean_return"`
	VenuePnl         float64  `json:"venue_pnl"`
}

/*
LifecycleReport is what the hub serves: every lifecycle, newest first, and the
performance of the closed ones.
*/
type LifecycleReport struct {
	Lifecycles  []Lifecycle `json:"lifecycles"`
	Performance Performance `json:"performance"`
}

func performance(lifecycles []Lifecycle) Performance {
	var perf Performance
	var shadowReturns, venueReturns float64
	venueTrades := 0

	for _, life := range lifecycles {
		outcome := life.Outcome

		if outcome == nil {
			continue
		}

		perf.Closed++

		if outcome.VenueCost > 0 {
			venueTrades++
			venueReturns += outcome.VenueRealized / outcome.VenueCost
			perf.VenuePnl += outcome.VenueRealized

			if outcome.VenueRealized > 0 {
				perf.VenueWins++
			}
		}

		if outcome.ShadowDefined && outcome.ShadowCost > 0 {
			perf.ShadowTrades++
			shadowReturns += outcome.ShadowRealized / outcome.ShadowCost
			perf.ShadowPnl += outcome.ShadowRealized

			if outcome.ShadowRealized > 0 {
				perf.ShadowWins++
			}
		}
	}

	ratio := func(value float64, count int) *float64 {
		if count == 0 {
			return nil
		}

		out := value / float64(count)
		return &out
	}

	perf.ShadowWinRate = ratio(float64(perf.ShadowWins), perf.ShadowTrades)
	perf.ShadowMeanReturn = ratio(shadowReturns, perf.ShadowTrades)
	perf.VenueWinRate = ratio(float64(perf.VenueWins), venueTrades)
	perf.VenueMeanReturn = ratio(venueReturns, venueTrades)

	return perf
}

/*
Lifecycles reports every position lifecycle of the session, newest first, and
the performance of the closed ones.
*/
func (desk *Desk) Lifecycles() LifecycleReport {
	if desk == nil {
		return LifecycleReport{Lifecycles: []Lifecycle{}}
	}

	desk.mu.Lock()
	out := make([]Lifecycle, 0, len(desk.lifecycles))

	for idx := len(desk.lifecycles) - 1; idx >= 0; idx-- {
		out = append(out, desk.lifecycles[idx].copy())
	}

	desk.mu.Unlock()

	return LifecycleReport{Lifecycles: out, Performance: performance(out)}
}

/*
LifecyclesReport is Lifecycles for the hub, which does not depend on broker
types.
*/
func (desk *Desk) LifecyclesReport() any {
	return desk.Lifecycles()
}

/*
Matched records a learned match on the symbol's open position: the exit
match before the exit it triggers. Without an open position it records
nothing and reports false.
*/
func (desk *Desk) Matched(symbol string, match Match) bool {
	desk.mu.Lock()
	defer desk.mu.Unlock()

	held, ok := desk.positions[symbol]

	if !ok || held.life == nil {
		return false
	}

	kind := EventExitMatch

	if match.Action == "enter" {
		kind = EventEntryMatch
	}

	held.life.record(kind, "learned "+match.Action+" matched on "+match.Path, match.At, matchFields(match))
	desk.lifeVersion()

	return true
}

func matchFields(match Match) map[string]any {
	return map[string]any{
		"action":     match.Action,
		"path":       match.Path,
		"tokens":     match.Tokens,
		"candidates": match.Candidates,
		"actions":    match.Actions,
		"confidence": match.Confidence,
		"threshold":  match.Threshold,
		"span_s":     match.Span.Seconds(),
	}
}

func (desk *Desk) lifeVersion() {
	desk.positionsVersion.Add(1)
}
