package strategy

import (
	"encoding/json"
	"fmt"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/data"
	nmstore "github.com/theapemachine/symm/nomagique/store"
)

/*
sequiturWire is the live grammar frame the learning dashboard reads
(frontend sequitur.ts). Epoch and timestamp are strings because they do not
fit a JavaScript number.
*/
type sequiturWire struct {
	Symbol         string              `json:"symbol"`
	Epoch          string              `json:"epoch"`
	Tick           uint64              `json:"tick"`
	Timestamp      string              `json:"timestamp"`
	Active         []sequiturRegion    `json:"active"`
	Midpoint       float64             `json:"midpoint"`
	Fired          []string            `json:"fired"`
	Held           int                 `json:"held"`
	Gate           *struct{}           `json:"gate"`
	Blocked        bool                `json:"blocked"`
	Events         []struct{}          `json:"events"`
	Completed      string              `json:"completed"`
	Path           string              `json:"path"`
	Policy         sequiturPolicyWire  `json:"policy"`
	Rules          []GrammarRuleExport `json:"rules"`
	Start          []string            `json:"start"`
	RawLength      int                 `json:"rawLength"`
	GrammarSymbols int                 `json:"grammarSymbols"`
	Violations     []string            `json:"violations"`
}

type sequiturRegion struct {
	Key        uint8   `json:"key"`
	Brightness float64 `json:"brightness"`
}

type sequiturPolicyWire struct {
	Epoch   string `json:"epoch"`
	Paths   int    `json:"paths"`
	Trading bool   `json:"trading"`
	Stored  bool   `json:"stored"`
	Match   string `json:"match"`
	Depth   int    `json:"depth"`
	Action  string `json:"action"`
	Outcome string `json:"outcome"`
}

/*
publishSequitur writes one grammar frame for symbol after a new chord.
The hub keeps the latest frame per symbol and replays it when a dashboard
connects. The Sequitur tab ingests origin sequitur, role grammar.
*/
func (training *Training) publishSequitur(
	tick *data.Value,
	symbol string,
	active []*nmstore.Region,
	midpoint float64,
	fired []string,
	held int,
	path string,
	completed *Rule,
	action, outcome string,
) error {
	if training == nil || training.hub == nil || symbol == "" || tick == nil {
		return nil
	}

	state := training.symbolState(symbol)
	export := training.CognitionGrammar(symbol)
	pol := training.policy.Load()
	paths := 0
	trading := false

	if pol != nil {
		paths = len(pol.keys)
		trading = paths > 0
	}

	frame := sequiturWire{
		Symbol:    symbol,
		Epoch:     fmt.Sprintf("%d", tick.Epoch),
		Tick:      tick.Tick,
		Timestamp: fmt.Sprintf("%d", tick.Timestamp),
		Active:    make([]sequiturRegion, 0, len(active)),
		Midpoint:  midpoint,
		Fired:     fired,
		Held:      held,
		Events:    []struct{}{},
		Path:      path,
		Policy: sequiturPolicyWire{
			Epoch:   fmt.Sprintf("%d", tick.Epoch),
			Paths:   paths,
			Trading: trading,
			Stored:  action != "",
			Match:   path,
			Depth:   len(fired),
			Action:  action,
			Outcome: outcome,
		},
		Rules:      export.Rules,
		Start:      export.Start,
		Violations: []string{},
	}

	if frame.Fired == nil {
		frame.Fired = []string{}
	}

	if frame.Rules == nil {
		frame.Rules = []GrammarRuleExport{}
	}

	if frame.Start == nil {
		frame.Start = []string{}
	}

	for _, region := range active {
		if region == nil {
			continue
		}

		frame.Active = append(frame.Active, sequiturRegion{Key: region.Key, Brightness: region.Brightness})
	}

	if completed != nil {
		frame.Completed = fmt.Sprintf("M%d", completed.ID())
	}

	if state.grammar != nil && state.grammar.startRule != nil {
		frame.RawLength = state.grammar.startRule.Length()
		frame.GrammarSymbols = frame.RawLength
	}

	for _, rule := range frame.Rules {
		frame.GrammarSymbols += rule.Length
	}

	buf, err := json.Marshal(frame)

	if err != nil {
		return errnie.Err(errnie.Internal, "[sequitur] frame of "+symbol, err)
	}

	out := data.NewValue(data.SequiturOrigin, data.SequiturRole, symbol)
	out.Epoch, out.Tick, out.Timestamp = tick.Epoch, tick.Tick, tick.Timestamp
	out.FormatPrefix()
	out.AppendJSON(buf, data.LayerGrammar)

	if err := training.writeFrame("sequitur/grammar/"+symbol, out); err != nil {
		return errnie.Err(errnie.IO, fmt.Sprintf("[sequitur] frame of %s at tick %d", symbol, tick.Tick), err)
	}

	return nil
}
