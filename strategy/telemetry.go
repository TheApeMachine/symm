package strategy

import (
	"context"
	"io"
	"sync"

	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/nomagique/runtime"
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/ui"
)

/*
Telemetry publishes training/step hub frames after Evaluate: observation
readouts and rollups driven only by layers Evaluate appended (gate, blocked,
policy, cut).
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

type gateFrame struct {
	Phase          string  `json:"phase"`
	Passed         bool    `json:"passed"`
	Virtual        bool    `json:"virtual"`
	Reason         string  `json:"reason,omitempty"`
	NetSpark       float64 `json:"netSpark"`
	NetHold        float64 `json:"netHold"`
	NetExit        float64 `json:"netExit"`
	TreeAction     string  `json:"treeAction,omitempty"`
	TreeVirtual    bool    `json:"treeVirtual,omitempty"`
	TreeExplore    bool    `json:"treeExplore,omitempty"`
	TreeBlocked    bool    `json:"treeBlocked,omitempty"`
	TreeImpulse    float64 `json:"treeImpulse,omitempty"`
	TreeResistance float64 `json:"treeResistance,omitempty"`
	PathBook       float64 `json:"pathBook,omitempty"`
}

/*
pathWalk is the live token path for one symbol. The dashboard gets a batch of
these once per tick, and only for symbols whose path changed.
*/
type pathWalk struct {
	Symbol      string `json:"symbol"`
	Phase       string `json:"phase"`
	Live        string `json:"live,omitempty"`
	Trigger     string `json:"trigger,omitempty"`
	Policy      string `json:"policy,omitempty"`
	Enter       bool   `json:"enter"`
	StoryPhase  string `json:"storyPhase,omitempty"`
	Outcome     string `json:"outcome,omitempty"`
}

type Telemetry struct {
	*runtime.System
	rollup    TickMetrics
	walks     []pathWalk
	lastPhase *sync.Map
}

func NewTelemetry(ctx context.Context) *Telemetry {
	return &Telemetry{
		System:    runtime.NewSystem(ctx, "telemetry"),
		lastPhase: &sync.Map{},
	}
}

func (telemetry *Telemetry) ResetTick() {
	telemetry.rollup = TickMetrics{}
}

func (telemetry *Telemetry) Publish() {
	if !uiActive() {
		return
	}

	rollup := telemetry.rollup
	telemetry.rollup = TickMetrics{}

	value := data.NewValue("training", "longtail", "aggregate")
	value.Append(rollup, "longtail")
	io.Copy(ui.UI, value)
}

/*
CompleteSymbolStep attaches policy, publishes the step frame, and updates
longtail rollups from gate/policy on the value.
*/
func (telemetry *Telemetry) CompleteSymbolStep(
	out *data.Value,
	model *Model,
) {
	if out == nil {
		return
	}

	telemetry.attachPolicy(out, model)
	telemetry.publishStep(out)
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
		out, model, pathKey, liveAfter, triggerAfter, policyPath, outcome, enter,
	), "policy")
}

func stepPolicyFromStep(
	out *data.Value,
	model *Model,
	pathKey, livePath, triggerPath, policyPath, outcome string,
	enter bool,
) stepPolicy {
	var phase string
	if out != nil {
		out.Lookup(&phase, "phase")
	}
	action := phase
	if action == "" {
		action = "enter"
		if !enter {
			action = "exit"
		}
	}

	stored := false

	pathPhase := policyPath
	if pathPhase == "" {
		pathPhase = livePath
	}

	depth := 0
	if pathPhase != "" {
		depth = 1
		for _, char := range pathPhase {
			if char == '/' {
				depth++
			}
		}
	}

	return stepPolicy{
		Trading:     model != nil && model.desk != nil,
		Stored:      stored,
		Match:       pathKey,
		LivePath:    livePath,
		TriggerPath: triggerPath,
		PolicyPath:  policyPath,
		Depth:       depth,
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

	var live, trigger, policy, outcome, storyPhase string
	var enter bool

	out.Lookup(&live, "livePath")
	out.Lookup(&trigger, "triggerPath")
	out.Lookup(&policy, "policyPath")
	out.Lookup(&outcome, "outcome")
	out.Lookup(&enter, "enter")
	out.Lookup(&storyPhase, "phase")

	phase := policy
	if phase == "" {
		phase = trigger
	}
	if phase == "" {
		phase = live
	}

	if telemetry.lastPhase == nil {
		telemetry.lastPhase = &sync.Map{}
	}

	lastPhase, _ := telemetry.lastPhase.LoadOrStore(symbol, phase)
	if outcome == "" && lastPhase == phase {
		return
	}

	telemetry.lastPhase.Store(symbol, phase)
	telemetry.walks = append(telemetry.walks, pathWalk{
		Symbol:     symbol,
		Phase:      phase,
		Live:       live,
		Trigger:    trigger,
		Policy:     policy,
		Enter:      enter,
		StoryPhase: storyPhase,
		Outcome:    outcome,
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

	rows := telemetry.walks
	telemetry.walks = nil

	if !uiActive() {
		telemetry.lastPhase = nil
		return
	}

	if len(rows) == 0 {
		return
	}

	io.Copy(ui.UI, data.NewValue("training", "walk", "aggregate").Append(rows, "walks"))
}

func (telemetry *Telemetry) EndTick(model *Model, desk *broker.Desk) {
	telemetry.enrichLedger(desk)
	telemetry.Publish()
}

func (telemetry *Telemetry) publishStep(out *data.Value) {
	if !uiActive() || out == nil {
		return
	}

	var lit []*nmstore.Region
	var cut tokenizer.Tokens
	out.Lookup(&lit, "lit")
	out.Lookup(&cut, "cut")

	fired, midpoint := litFired(cut.Last())

	out.Append(regionActive(lit), "active")
	out.Append(midpoint, "midpoint")
	out.Append(fired, "fired")

	telemetry.recordStep(out)
	io.Copy(ui.UI, out)
}

func (telemetry *Telemetry) recordStep(out *data.Value) {
	var gate stepGate
	var gateFrame gateFrame
	var policy stepPolicy
	var blocked bool
	var blockedReason string
	var lit []*nmstore.Region
	var outcome string
	var paperOutcome string

	out.Lookup(&gateFrame, "gate")
	if gateFrame.Phase != "" {
		gate.Passed = gateFrame.Passed
	}
	out.Lookup(&gate, "gate")
	if !gate.Passed && gateFrame.Passed {
		gate.Passed = gateFrame.Passed
	}
	out.Lookup(&policy, "policy")
	out.Lookup(&blocked, "blocked")
	out.Lookup(&blockedReason, "blockedReason")
	out.Lookup(&lit, "lit")
	out.Lookup(&outcome, "outcome")
	out.Lookup(&paperOutcome, "paperOutcome")

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
	case "enter submitted", "virtual enter submitted", "paper enter submitted":
		telemetry.rollup.OutcomeEnter++
		if outcome == "virtual enter submitted" {
			telemetry.rollup.ExploreEnter++
		}
	case "exit submitted":
		telemetry.rollup.OutcomeExit++
	}

	if paperOutcome == "paper enter submitted" && outcome != "paper enter submitted" {
		telemetry.rollup.OutcomeEnter++
	}

	var phase string
	out.Lookup(&phase, "phase")
	var action string
	out.Lookup(&action, "action")
	if phase == "hold" && action == "exit" && outcome == "" && !blocked {
		telemetry.rollup.PolicyChecks++
	}
}

func (telemetry *Telemetry) enrichLedger(desk *broker.Desk) {
	if telemetry == nil || desk == nil {
		return
	}

	telemetry.rollup.VenueOpen = len(desk.OpenPositions(false))
	telemetry.rollup.VirtualExcursion = len(desk.OpenPositions(true))
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
