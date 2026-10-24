package strategy

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/ui"
)

/*
Telemetry publishes training/step hub frames after Evaluate: grammar state,
observation readouts, and rollups driven only by layers Evaluate appended
(gate, blocked, policy, cut).
*/
type stepGate struct {
	Surprise *float64 `json:"surprise"`
	Z        *float64 `json:"z,omitempty"`
	Warm     bool     `json:"warm"`
	Maturity float64  `json:"maturity,omitempty"`
	Cold     string   `json:"cold,omitempty"`
	Passed   bool     `json:"passed"`
	Ignition bool     `json:"ignition,omitempty"`
}

type stepPolicy struct {
	Epoch       string `json:"epoch"`
	Paths       int    `json:"paths"`
	Trading     bool   `json:"trading"`
	Stored      bool   `json:"stored"`
	Match       string `json:"match"`
	LivePath    string `json:"livePath"`
	TriggerPath string `json:"triggerPath,omitempty"`
	PolicyPath  string `json:"policyPath,omitempty"`
	Depth       int    `json:"depth"`
	Action      string `json:"action"`
	Outcome     string `json:"outcome"`
}

/*
pathWalk is the sentence one symbol is spelling. The dashboard gets a batch
of these once per tick, and only for symbols whose sentence changed.
*/
type pathWalk struct {
	Symbol  string `json:"symbol"`
	Phase   string `json:"phase"`
	Live    string `json:"live,omitempty"`
	Trigger string `json:"trigger,omitempty"`
	Policy  string `json:"policy,omitempty"`
	Enter   bool   `json:"enter"`
	Outcome string `json:"outcome,omitempty"`
}

type Telemetry struct {
	*runtime.System
	rollupMu  sync.Mutex
	rollup    TickMetrics
	walkMu    sync.Mutex
	walks     []pathWalk
	lastPhase map[string]string
}

func NewTelemetry(ctx context.Context) *Telemetry {
	return &Telemetry{
		System: runtime.NewSystem(ctx, "telemetry"),
	}
}

func (telemetry *Telemetry) ResetTick() {
	telemetry.rollupMu.Lock()
	telemetry.rollup = TickMetrics{}
	telemetry.rollupMu.Unlock()
}

func (telemetry *Telemetry) Publish() {
	if !uiActive() {
		return
	}

	telemetry.rollupMu.Lock()
	rollup := telemetry.rollup
	telemetry.rollup = TickMetrics{}
	telemetry.rollupMu.Unlock()

	value := data.NewValue("training", "longtail", "aggregate")
	value.Append(rollup, "longtail")
	io.Copy(ui.UI, value)
}

/*
CompleteSymbolStep attaches policy, enriches grammar layers from cut/lit, publishes
the step frame, and updates longtail rollups from gate/policy on the value.
*/
func (telemetry *Telemetry) CompleteSymbolStep(
	out *data.Value,
	sequencer *tokenizer.Sequencer,
	model *Model,
) {
	if out == nil {
		return
	}

	telemetry.attachPolicy(out, model)
	telemetry.publishStep(out, sequencer)
}

func (telemetry *Telemetry) attachPolicy(out *data.Value, model *Model) {
	if model == nil {
		return
	}

	var (
		outcome      string
		pathKey      string
		liveAfter    string
		triggerAfter string
		policyPath   string
		enter        bool
	)

	out.Lookup(&outcome, "outcome")
	out.Lookup(&pathKey, "path")
	out.Lookup(&liveAfter, "livePath")
	out.Lookup(&triggerAfter, "triggerPath")
	out.Lookup(&policyPath, "policyPath")
	out.Lookup(&enter, "enter")

	out.Append(stepPolicyFromStep(
		model, pathKey, liveAfter, triggerAfter, policyPath, outcome, enter,
	), "policy")
}

func stepPolicyFromStep(
	model *Model,
	pathKey, livePath, triggerPath, policyPath, outcome string,
	enter bool,
) stepPolicy {
	action := "enter"
	if !enter {
		action = "exit"
	}

	stored := false
	if model != nil && pathKey != "" {
		key := []byte(pathKey)
		model.trieMu.RLock()
		if _, ok := model.explore.Get(key); ok {
			stored = true
		} else if _, ok := model.policy.Get(key); ok {
			stored = true
		}
		model.trieMu.RUnlock()
	}

	phase := policyPath
	if phase == "" {
		phase = livePath
	}

	return stepPolicy{
		Trading:     model != nil && model.desk != nil,
		Stored:      stored,
		Match:       pathKey,
		LivePath:    livePath,
		TriggerPath: triggerPath,
		PolicyPath:  policyPath,
		Depth:       pathSegments(phase),
		Action:      action,
		Outcome:     outcome,
	}
}

/*
EndTick adds open desk counts to the rollup and publishes training/longtail.
*/
func (telemetry *Telemetry) NoteStep(out *data.Value) {
	if telemetry == nil || out == nil {
		return
	}

	telemetry.recordStep(out)
}

/*
NoteWalk keeps a symbol whose phase path or outcome changed. Vetoed ticks
that repeat the same sentence are not queued.
*/
func (telemetry *Telemetry) NoteWalk(symbol string, out *data.Value) {
	if telemetry == nil || out == nil || symbol == "" {
		return
	}

	var live, trigger, policy, outcome string
	var enter bool

	out.Lookup(&live, "livePath")
	out.Lookup(&trigger, "triggerPath")
	out.Lookup(&policy, "policyPath")
	out.Lookup(&outcome, "outcome")
	out.Lookup(&enter, "enter")

	phase := policy
	if phase == "" {
		phase = trigger
	}
	if phase == "" {
		phase = live
	}

	telemetry.walkMu.Lock()
	defer telemetry.walkMu.Unlock()

	if telemetry.lastPhase == nil {
		telemetry.lastPhase = map[string]string{}
	}

	if outcome == "" && telemetry.lastPhase[symbol] == phase {
		return
	}

	telemetry.lastPhase[symbol] = phase
	telemetry.walks = append(telemetry.walks, pathWalk{
		Symbol:  symbol,
		Phase:   phase,
		Live:    live,
		Trigger: trigger,
		Policy:  policy,
		Enter:   enter,
		Outcome: outcome,
	})
}

/*
PublishWalks sends the tick's changed sentences as one frame. With no
dashboard attached the batch is dropped and the next attach starts clean.
*/
func (telemetry *Telemetry) PublishWalks() {
	if telemetry == nil {
		return
	}

	telemetry.walkMu.Lock()
	rows := telemetry.walks
	telemetry.walks = nil

	if !uiActive() {
		telemetry.lastPhase = nil
		telemetry.walkMu.Unlock()
		return
	}

	telemetry.walkMu.Unlock()

	if len(rows) == 0 {
		return
	}

	io.Copy(ui.UI, data.NewValue("training", "walk", "aggregate").Append(rows, "walks"))
}

func (telemetry *Telemetry) EndTick(model *Model, desk *broker.Desk) {
	telemetry.enrichLedger(desk)
	telemetry.noteLearning(model)
	telemetry.Publish()
}

// publishStep appends grammar layers onto the training step value and publishes it.
func (telemetry *Telemetry) publishStep(
	out *data.Value,
	sequencer *tokenizer.Sequencer,
) {
	if !uiActive() || out == nil {
		return
	}

	if sequencer != nil && sequencer.Grammar() != nil {
		var lit []*nmstore.Region
		var cut tokenizer.Token
		out.Lookup(&lit, "lit")
		out.Lookup(&cut, "cut")

		fired, midpoint := litFired(cut)

		out.Append(regionActive(lit), "active")
		out.Append(midpoint, "midpoint")
		out.Append(fired, "fired")

		events := []map[string]any{}
		if len(cut.Symbols) > 0 {
			kind := sequencer.Grammar().LastKind()
			if kind == "" {
				kind = "append"
			}

			events = append(events, map[string]any{
				"kind":   kind,
				"token":  cut.String(),
				"rule":   0,
				"into":   0,
				"from":   0,
				"digram": []string{"", ""},
			})
		}

		completed := ""
		if len(cut.Symbols) > 0 {
			if rule := sequencer.Grammar().LastRule(); rule != nil {
				completed = tokenizer.RuleRef(rule.ID)
			}
		}

		out.Append(events, "grammar", "events")
		out.Append(completed, "grammar", "completed")
		out.Append(grammarRules(sequencer), "grammar", "rules")
		out.Append(grammarStart(sequencer), "grammar", "start")
		out.Append(sequencer.Grammar().RootLength(), "grammar", "rawLength")
		out.Append(sequencer.Grammar().Symbols(), "grammar", "grammarSymbols")
	}

	telemetry.recordStep(out)
	io.Copy(ui.UI, out)
}

func (telemetry *Telemetry) recordStep(out *data.Value) {
	var gate stepGate
	var policy stepPolicy
	var blocked bool
	var blockedReason string
	var lit []*nmstore.Region
	var outcome string

	out.Lookup(&gate, "gate")
	out.Lookup(&policy, "policy")
	out.Lookup(&blocked, "blocked")
	out.Lookup(&blockedReason, "blockedReason")
	out.Lookup(&lit, "lit")
	out.Lookup(&outcome, "outcome")

	telemetry.rollupMu.Lock()
	defer telemetry.rollupMu.Unlock()

	telemetry.rollup.Symbols++
	telemetry.rollup.GateTotal++
	if len(lit) > 0 {
		telemetry.rollup.LitTicks++
	}
	if gate.Passed {
		telemetry.rollup.GatePass++
	}
	if blocked {
		telemetry.rollup.Blocked++
		switch blockedReason {
		case "brightness":
			telemetry.rollup.BlockedBrightness++
		case "duplicate":
			telemetry.rollup.BlockedDuplicate++
		case "veto":
			telemetry.rollup.BlockedVeto++
		case "empty":
			telemetry.rollup.BlockedEmpty++
		}
	}

	telemetry.rollup.PolicyChecks++
	if policy.Stored {
		telemetry.rollup.PolicyStored++
	}

	switch outcome {
	case "enter submitted":
		telemetry.rollup.OutcomeEnter++
		telemetry.rollup.ExploreEnter++
	case "exit submitted":
		telemetry.rollup.OutcomeExit++
	}
}

func (telemetry *Telemetry) enrichLedger(desk *broker.Desk) {
	if telemetry == nil || desk == nil {
		return
	}

	telemetry.rollupMu.Lock()
	defer telemetry.rollupMu.Unlock()

	telemetry.rollup.VenueOpen = len(desk.OpenPositions(false))
	telemetry.rollup.VirtualExcursion = len(desk.OpenPositions(true))
}

func (telemetry *Telemetry) noteLearning(model *Model) {
	if telemetry == nil || model == nil {
		return
	}

	snap := model.LearningSnapshot()

	telemetry.rollupMu.Lock()
	defer telemetry.rollupMu.Unlock()

	telemetry.rollup.DevelopedPaths = snap.Developed
	telemetry.rollup.PromotedPaths = snap.Promoted
	telemetry.rollup.PrunedPaths = snap.Pruned
	telemetry.rollup.MeanThreshold = snap.MeanThreshold
	telemetry.rollup.ExitProfit = snap.ExitProfit
	telemetry.rollup.ExitStop = snap.ExitStop
	telemetry.rollup.ExitBranch = snap.ExitBranch
}

func grammarRules(sequencer *tokenizer.Sequencer) []map[string]any {
	grammar := sequencer.Grammar()
	if grammar == nil {
		return []map[string]any{}
	}

	views := grammar.RuleViews()
	rules := make([]map[string]any, 0, len(views))

	for _, view := range views {
		symbols := view.Symbols
		if symbols == nil {
			symbols = []string{}
		}
		terminals := view.Terminals
		if terminals == nil {
			terminals = []string{}
		}

		rules = append(rules, map[string]any{
			"id":        view.ID,
			"name":      tokenizer.RuleRef(view.ID),
			"count":     view.Count,
			"depth":     view.Depth,
			"length":    len(symbols),
			"symbols":   symbols,
			"terminals": terminals,
		})
	}

	return rules
}

func grammarStart(sequencer *tokenizer.Sequencer) []string {
	grammar := sequencer.Grammar()
	if grammar == nil {
		return []string{}
	}

	start := grammar.RootSequence()
	if start == nil {
		return []string{}
	}

	return start
}

func litFired(cut tokenizer.Token) ([]string, float64) {
	if len(cut.Symbols) == 0 {
		return []string{}, 0
	}

	fired := append([]string(nil), cut.Symbols...)
	return fired, cut.Brightness
}

func regionActive(lit []*nmstore.Region) []map[string]any {
	active := make([]map[string]any, 0, len(lit))

	for _, region := range lit {
		if region == nil {
			continue
		}

		active = append(active, map[string]any{
			"key":        region.Key,
			"brightness": region.Brightness,
		})
	}

	if active == nil {
		return []map[string]any{}
	}

	return active
}

func uiActive() bool {
	return ui.UI != nil && ui.UI.Active()
}
