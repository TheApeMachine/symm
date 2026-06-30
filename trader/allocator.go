package trader

import (
	"math"
	"sort"
	"strings"

	"github.com/spf13/viper"
	"github.com/theapemachine/datura"
	"github.com/theapemachine/symm/logic"
)

type Allocator struct {
	fraction       float64
	quote          string
	pendingEntries int
}

func NewAllocator() *Allocator {
	return &Allocator{
		fraction: viper.GetFloat64("trading.sizing.base_fraction"),
		quote: strings.ToUpper(
			viper.GetString("market.quote_currency"),
		),
	}
}

func (allocator *Allocator) SetPendingEntries(count int) {
	if count < 0 {
		count = 0
	}

	allocator.pendingEntries = count
}

/*
Allowed stamps the complete candidate set with allocator verdicts and returns it
in dispatch order. The broker refuses candidates stamped allowed=false, so
blocked entries remain visible to UI/audit without reaching Kraken.
*/
func (allocator *Allocator) Allowed(
	actions []*datura.Artifact, balances *datura.Artifact,
) []*datura.Artifact {
	if len(actions) == 0 {
		return actions
	}

	ledger := newRiskLedger(allocator, balances)

	exits := make([]*datura.Artifact, 0, len(actions))
	entries := make([]*datura.Artifact, 0, len(actions))

	for _, action := range actions {
		if action == nil {
			continue
		}

		if isExit(action) {
			stampRisk(action, true, 0, 0, ledger.availableQuote, "")
			exits = append(exits, action)
			continue
		}

		entries = append(entries, action)
	}

	sort.SliceStable(entries, func(first, second int) bool {
		firstScore := datura.Peek[float64](entries[first], "decision", "score")
		secondScore := datura.Peek[float64](entries[second], "decision", "score")

		return firstScore > secondScore
	})

	out := make([]*datura.Artifact, 0, len(exits)+len(entries))
	out = append(out, exits...)

	for _, action := range entries {
		symbol, _ := action.Scope()
		confidence := allocationConfidence(action)
		fraction := allocator.calculate(confidence)
		notional := ledger.notionalFor(action, fraction)
		reason := ledger.admit(symbol, action, notional)

		if reason != "" {
			stampRisk(action, false, fraction, notional, ledger.availableQuote, reason)
			stampVerdict(action, "blocked", reason, datura.Peek[float64](action, "decision", "score"))
			out = append(out, action)
			continue
		}

		reserve := ledger.reserveFor(action, notional)
		ledger.availableQuote -= reserve
		ledger.pendingEntries++
		ledger.acceptedEntries++
		if logic.ActionType(datura.Peek[string](action, "type")).Protective() {
			ledger.acceptedOpportunity++
		} else if datura.Peek[bool](action, "opportunity_slot") {
			ledger.acceptedOpportunity++
		} else {
			ledger.acceptedNormal++
		}

		availableAfter := ledger.availableQuote
		if availableAfter < 0 {
			availableAfter = 0
		}

		stampRisk(action, true, fraction, notional, availableAfter, "")
		out = append(out, action)
	}

	return out
}

func allocationConfidence(action *datura.Artifact) float64 {
	for _, path := range [][]any{
		{"decision", "confidence"},
		{"entry_confidence"},
		{"confidence"},
	} {
		confidence := datura.Peek[float64](action, path...)
		if confidence > 0 {
			return confidence
		}
	}

	return 0
}

/*
calculate sizes an admitted entry by wallet policy: base wallet risk scaled by
backend confidence. A non-finite confidence fails closed to zero.
*/
func (allocator *Allocator) calculate(confidence float64) float64 {
	if confidence <= 0 || math.IsNaN(confidence) || math.IsInf(confidence, 0) {
		return 0
	}

	return allocator.fraction * confidence
}

type riskLedger struct {
	quote               string
	quoteCash           float64
	availableQuote      float64
	openPositions       int
	held                map[string]bool
	pendingEntries      int
	acceptedEntries     int
	acceptedNormal      int
	acceptedOpportunity int
	maxConcurrent       int
	normalSlots         int
	opportunitySlots    int
	maxOrderNotional    float64
	maxDailyLoss        float64
	realizedDailyLoss   float64
	edgeMin             float64
	defaultFriction     float64
}

func newRiskLedger(allocator *Allocator, balances *datura.Artifact) *riskLedger {
	quote := "USD"
	if allocator != nil && allocator.quote != "" {
		quote = allocator.quote
	}

	ledger := &riskLedger{
		quote:             quote,
		held:              make(map[string]bool),
		maxConcurrent:     viper.GetInt("trading.max_concurrent_positions"),
		normalSlots:       viper.GetInt("trading.slots.normal"),
		opportunitySlots:  viper.GetInt("trading.entry.opportunity_slot_count"),
		maxOrderNotional:  viper.GetFloat64("live.max_order_notional"),
		maxDailyLoss:      viper.GetFloat64("live.max_daily_loss"),
		realizedDailyLoss: realizedDailyLoss(),
		edgeMin:           viper.GetFloat64("trading.edge_min_bps") / 10_000,
		defaultFriction:   defaultRoundTripFriction(),
	}
	if allocator != nil {
		ledger.pendingEntries = allocator.pendingEntries
	}

	if ledger.maxConcurrent <= 0 {
		ledger.maxConcurrent = math.MaxInt
	}
	if ledger.normalSlots <= 0 {
		ledger.normalSlots = ledger.maxConcurrent
	}
	if ledger.opportunitySlots < 0 {
		ledger.opportunitySlots = 0
	}

	for _, row := range balanceRows(balances) {
		asset := strings.ToUpper(strings.TrimSpace(row.Asset))
		if asset == "" || row.Balance <= 0 {
			continue
		}

		if asset == quote {
			ledger.quoteCash = row.Balance
			ledger.availableQuote = row.Balance
			continue
		}

		ledger.openPositions++
		ledger.held[asset] = true
		ledger.held[asset+"/"+quote] = true
	}

	return ledger
}

func (ledger *riskLedger) admit(symbol string, action *datura.Artifact, notional float64) string {
	if ledger == nil {
		return "risk_unavailable"
	}

	if symbol != "" && ledger.held[strings.ToUpper(symbol)] {
		return "held"
	}

	if ledger.maxDailyLoss > 0 && ledger.realizedDailyLoss >= ledger.maxDailyLoss {
		return "max_daily_loss"
	}

	if ledger.openPositions+ledger.pendingEntries >= ledger.maxConcurrent {
		return "max_positions"
	}

	opportunity := datura.Peek[bool](action, "opportunity_slot")
	if opportunity {
		if ledger.acceptedOpportunity >= ledger.opportunitySlots {
			return "slot_exhausted"
		}
	} else if ledger.acceptedNormal >= ledger.normalSlots {
		return "slot_exhausted"
	}

	if notional <= 0 || math.IsNaN(notional) || math.IsInf(notional, 0) {
		return "insufficient_cash"
	}

	if ledger.maxOrderNotional > 0 && notional > ledger.maxOrderNotional {
		return "max_order_notional"
	}

	if ledger.reserveFor(action, notional) > ledger.availableQuote {
		return "insufficient_cash"
	}

	return ""
}

func (ledger *riskLedger) notionalFor(action *datura.Artifact, fraction float64) float64 {
	for _, path := range [][]any{
		{"notional"},
		{"decision", "notional"},
		{"risk", "notional"},
	} {
		notional := datura.Peek[float64](action, path...)
		if notional > 0 && !math.IsNaN(notional) && !math.IsInf(notional, 0) {
			return notional
		}
	}

	if fraction <= 0 || ledger == nil || ledger.quoteCash <= 0 {
		return 0
	}

	return ledger.quoteCash * fraction
}

func (ledger *riskLedger) reserveFor(action *datura.Artifact, notional float64) float64 {
	if ledger == nil || notional <= 0 {
		return 0
	}

	friction := datura.Peek[float64](action, "decision", "friction")
	if friction <= 0 {
		friction = datura.Peek[float64](action, "decision", "friction_bps") / 10_000
	}
	if friction <= 0 {
		friction = ledger.defaultFriction
	}
	if friction < 0 || math.IsNaN(friction) || math.IsInf(friction, 0) {
		friction = 0
	}

	return notional * (1 + friction + ledger.edgeMin)
}

func stampRisk(
	action *datura.Artifact,
	allowed bool,
	fraction float64,
	notional float64,
	availableAfter float64,
	reason string,
) {
	if action == nil {
		return
	}

	action.Poke(allowed, "allowed").
		Poke(fraction, "fraction").
		Poke(notional, "notional").
		Poke(reason, "risk", "reason").
		Poke(availableAfter, "risk", "available_after")

	if reason != "" {
		action.Poke(reason, "why")
	}
}

func defaultRoundTripFriction() float64 {
	taker := viper.GetFloat64("trading.paper.taker_fee_bps")
	slippage := viper.GetFloat64("trading.paper.slippage_bps")
	roundTrip := 2*taker + 2*slippage
	if roundTrip <= 0 || math.IsNaN(roundTrip) || math.IsInf(roundTrip, 0) {
		return 0
	}

	return roundTrip / 10_000
}

func realizedDailyLoss() float64 {
	for _, key := range []string{
		"trading.realized_daily_loss",
		"trading.daily_loss",
		"live.realized_daily_loss",
		"live.daily_loss",
	} {
		value := viper.GetFloat64(key)
		if value > 0 {
			return value
		}
	}

	return 0
}
