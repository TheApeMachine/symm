package ui

import (
	"strings"

	"github.com/theapemachine/symm/market"
)

/*
Message is one typed dashboard update.
Only one field should be populated for each websocket write.
*/
type Message struct {
	Tick        *Tick          `json:"tick,omitempty"`
	Regime      *Regime        `json:"regime,omitempty"`
	Manifold    map[string]any `json:"manifold,omitempty"`
	Resonance   map[string]any `json:"resonance,omitempty"`
	Measurement *Measurement   `json:"measurement,omitempty"`
	Cognitive   *Cognitive     `json:"cognitive,omitempty"`
	Decision    *Decision      `json:"decision,omitempty"`
	Diagnostic  *Diagnostic    `json:"diagnostic,omitempty"`
	Balances    *Balances      `json:"balances,omitempty"`
	Orders      *Orders        `json:"orders,omitempty"`
	Executions  *Executions    `json:"executions,omitempty"`
	Positions   *Positions     `json:"positions,omitempty"`
}

type Tick struct {
	Count        int64  `json:"count"`
	Phase        string `json:"phase"`
	Measurements int    `json:"measurements"`
	Candidates   int    `json:"candidates"`
	At           string `json:"at"`
}

type Regime struct {
	Volatility float64 `json:"volatility"`
	Trend      float64 `json:"trend"`
	Bullish    float64 `json:"bullish"`
	Bearish    float64 `json:"bearish"`
	Choppiness float64 `json:"choppiness"`
	Observed   int     `json:"observed"`
	Confidence float64 `json:"confidence"`
	Strength   float64 `json:"strength"`
	At         string  `json:"at"`
}

type Measurement struct {
	Source        string             `json:"source"`
	Symbol        string             `json:"symbol"`
	At            string             `json:"at"`
	Category      string             `json:"category"`
	Confidence    float64            `json:"confidence"`
	Strength      float64            `json:"strength"`
	Surprise      float64            `json:"surprise"`
	EntryBaseline float64            `json:"entryBaseline"`
	ExitBaseline  float64            `json:"exitBaseline"`
	Status        string             `json:"status"`
	Distribution  map[string]float64 `json:"distribution"`
	Metrics       map[string]float64 `json:"metrics"`
}

type Cognitive struct {
	Readings  map[string]market.CognitiveReading `json:"readings"`
	At        string                             `json:"at"`
	UpdatedAt int64                              `json:"updatedAt"`
}

type Decision struct {
	ID              string  `json:"id"`
	Tick            int64   `json:"tick"`
	Symbol          string  `json:"symbol"`
	Type            string  `json:"type"`
	Side            string  `json:"side"`
	Verdict         string  `json:"verdict"`
	Reason          string  `json:"reason"`
	Score           float64 `json:"score"`
	EntryScore      float64 `json:"entryScore"`
	EntryConfidence float64 `json:"entryConfidence"`
	Fraction        float64 `json:"fraction"`
	Quantity        float64 `json:"quantity"`
	Notional        float64 `json:"notional"`
	ActionID        string  `json:"actionID"`
	DecisionID      string  `json:"decisionID"`
	BranchKey       string  `json:"branchKey"`
	ReasonSource    string  `json:"reasonSource"`
	ReasonCategory  string  `json:"reasonCategory"`
	DecisionAt      string  `json:"decisionAt"`
}

type Diagnostic struct {
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
	Symbol   string `json:"symbol"`
	At       string `json:"at"`
}

type Balances struct {
	Rows  []map[string]any `json:"rows"`
	Count int              `json:"count"`
	At    string           `json:"at"`
}

type Orders struct {
	Rows  []map[string]any `json:"rows"`
	Count int              `json:"count"`
	At    string           `json:"at"`
}

type Executions struct {
	Rows  []map[string]any `json:"rows"`
	Count int              `json:"count"`
	At    string           `json:"at"`
}

type Positions struct {
	Positions []map[string]any `json:"positions"`
	Count     int              `json:"count"`
	Quote     string           `json:"quote"`
	Net       float64          `json:"net"`
	At        string           `json:"at"`
}

func (message Message) Empty() bool {
	return message.Tick == nil &&
		message.Regime == nil &&
		message.Manifold == nil &&
		message.Resonance == nil &&
		message.Measurement == nil &&
		message.Cognitive == nil &&
		message.Decision == nil &&
		message.Diagnostic == nil &&
		message.Balances == nil &&
		message.Orders == nil &&
		message.Executions == nil &&
		message.Positions == nil
}

func (message Message) Key() string {
	if message.Tick != nil {
		return "tick"
	}

	if message.Regime != nil {
		return "regime"
	}

	if message.Manifold != nil {
		return "manifold"
	}

	if message.Resonance != nil {
		return "resonance"
	}

	if message.Measurement != nil {
		return strings.Join([]string{"measurement", message.Measurement.Source}, "/")
	}

	if message.Cognitive != nil {
		return "cognitive"
	}

	if message.Decision != nil {
		return strings.Join([]string{"decision", message.Decision.Symbol}, "/")
	}

	if message.Diagnostic != nil {
		return strings.Join([]string{"diagnostic", message.Diagnostic.Symbol}, "/")
	}

	if message.Balances != nil {
		return "balances"
	}

	if message.Orders != nil {
		return "orders"
	}

	if message.Executions != nil {
		return "executions"
	}

	if message.Positions != nil {
		return "positions"
	}

	return "message"
}
