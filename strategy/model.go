package strategy

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/nomagique/runtime"
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

type Model struct {
	*runtime.System
	desk  *broker.Desk
	price *broker.Price
	stage *Stage
	store *store.Client
	tree  *atomic.Pointer[Tree]
	gate  *Gate
}

func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	price *broker.Price,
	stage *Stage,
) *Model {
	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

	model := &Model{
		System: runtime.NewSystem(ctx, "model"),
		desk:   desk,
		price:  price,
		stage:  stage,
		store:  client,
		tree:   &atomic.Pointer[Tree]{},
		gate:   NewGate(ctx, desk, price, client),
	}

	tree := NewTree(ctx, "model/paths/")
	model.tree.Store(tree)

	return model
}

func (model *Model) Evaluate(
	out *data.Value, observed []*nmstore.Region, tick *data.Value,
) *data.Value {
	if out == nil {
		return nil
	}

	symbol := out.Scope
	source := out

	if tick != nil {
		source = tick
	}

	coder := ReadCoder(source, symbol)
	out.Append(observed, "lit")

	if tick == nil {
		return out
	}

	if pending, ok := model.desk.TakePending(symbol); ok {
		model.completeExit(out, symbol, pending)
	}

	holding := model.desk.Has(symbol, false) || model.desk.Has(symbol, true)

	step := data.Get[tokenizer.StepResult](
		source, strings.Join([]string{"stage", symbol, "step"}, "/"),
	)

	cut := data.Get[tokenizer.Token](
		source, strings.Join([]string{"stage", symbol, "cut"}, "/"),
	)

	if cut.String() == "" && step.Token.String() != "" {
		cut = step.Token
	}

	if cut.String() == "" && !holding {
		audit.Record.Write(map[string]any{
			"model":   "evaluate",
			"symbol":  symbol,
			"outcome": "empty-cut",
		})
		return out
	}

	out.Append(cut, "cut")

	if holding {
		return model.evaluateHold(out, symbol, observed, source, step, cut, coder)
	}

	return model.evaluateEnter(out, symbol, observed, step, cut, coder)
}

func (model *Model) evaluateEnter(
	out *data.Value,
	symbol string,
	observed []*nmstore.Region,
	step tokenizer.StepResult,
	cut tokenizer.Token,
	coder CoderReading,
) *data.Value {
	spell := tokenizer.CollapseConsecutive(step.Spell)
	pathKey := strings.Join(spell, "/")

	minPath := 2
	if system.Cfg != nil && system.Cfg.Learning != nil && system.Cfg.Learning.MinPathLength > 0 {
		minPath = system.Cfg.Learning.MinPathLength
	}

	// A stable letter (the sequencer marks the repeat) is a confirmed spell.
	// Waiting for a second distinct label left the book flat on a steady tape.
	confirmed := len(spell) >= minPath ||
		(len(spell) > 0 && step.Silence == tokenizer.StepSilenceDuplicate)
	if !confirmed {
		return out
	}

	out.Append(pathKey, "path")
	out.Append(pathKey, "policyPath")
	out.Append("enter", "phase")

	tokens := tokensFromSpell(spell, cut)
	decision := model.tree.Load().PredictDefault(tokens, coder, ActionEnter)
	out.Append(decision.Action.String(), "action")

	switch decision.Action {
	case ActionEnter:
		out.Append(true, "enter")

		gateDec := model.gate.EvaluateEnter(symbol, observed, coder)
		out.Append(model.gate.frameForEnter(observed, gateDec, decision), "gate")

		if !gateDec.Allowed {
			errnie.Warn("[model.evaluateEnter] gate blocked", "symbol", symbol, "reason", gateDec.Reason, "coder", coder)
			out.Append(true, "blocked")
			out.Append(gateDec.Reason, "blockedReason")
			return out
		}

		isVirtual := decision.Virtual || gateDec.Virtual

		if model.desk.Has(symbol, isVirtual) {
			return out
		}

		if err := model.desk.Enter(symbol, pathKey, isVirtual, true); err != nil {
			audit.Record.Write(map[string]any{
				"model":   "enter",
				"symbol":  symbol,
				"virtual": isVirtual,
				"path":    pathKey,
				"outcome": "rejected",
				"err":     err.Error(),
			})
			return out
		}

		outcome := "paper enter submitted"
		if isVirtual {
			outcome = "virtual enter submitted"
		}
		out.Append(outcome, "outcome")
		audit.Record.Write(map[string]any{
			"model":   "enter",
			"symbol":  symbol,
			"virtual": isVirtual,
			"path":    pathKey,
			"outcome": outcome,
		})

		if pos := model.desk.Position(symbol, isVirtual); pos != nil {
			pos.OpenSurprise = coder.Surprise
			pos.ExitPath = pathKey
		}

		if model.stage != nil {
			model.stage.ResetSequencer(symbol)
		}

	case ActionExit, ActionHold:
		out.Append(false, "enter")
	}

	return out
}

func (model *Model) evaluateHold(
	out *data.Value,
	symbol string,
	observed []*nmstore.Region,
	source *data.Value,
	step tokenizer.StepResult,
	cut tokenizer.Token,
	coder CoderReading,
) *data.Value {
	holdSpell := tokenizer.CollapseConsecutive(step.Spell)
	holdPath := strings.Join(holdSpell, "/")

	exitStep := data.Get[tokenizer.StepResult](
		source, strings.Join([]string{"stage", symbol, "exitStep"}, "/"),
	)
	exitCut := data.Get[tokenizer.Token](
		source, strings.Join([]string{"stage", symbol, "exitCut"}, "/"),
	)

	exitSpell := tokenizer.CollapseConsecutive(exitStep.Spell)
	exitPath := strings.Join(exitSpell, "/")

	out.Append(holdPath, "holdPath")
	out.Append(exitPath, "path")
	out.Append(exitPath, "policyPath")
	out.Append("hold", "phase")

	for _, virtual := range []bool{false, true} {
		if pos := model.desk.Position(symbol, virtual); pos != nil {
			pos.HoldPath = holdPath
			pos.ExitPath = exitPath
		}
	}

	holdTokens := tokensFromSpell(holdSpell, cut)
	exitTokens := tokensFromSpell(exitSpell, exitCut)

	holdDecision := model.tree.Load().PredictDefault(holdTokens, coder, ActionHold)
	exitDecision := model.tree.Load().PredictDefault(exitTokens, coder, ActionExit)

	out.Append(holdDecision.Action.String(), "holdAction")
	out.Append(exitDecision.Action.String(), "action")
	out.Append(model.gate.frameForHold(observed, holdDecision), "gate")

	if len(exitSpell) >= system.Cfg.Learning.MinPathLength &&
		exitDecision.Action == ActionExit &&
		exitDecision.Impulse >= exitDecision.Resistance {
		for _, virtual := range []bool{false, true} {
			if pos := model.desk.Position(symbol, virtual); pos != nil {
				gateDec := model.gate.EvaluateExit(symbol, observed, coder, pos)
				out.Append(model.gate.frameForExit(observed, gateDec, exitDecision), "gate")

				if !gateDec.Allowed {
					out.Append(true, "blocked")
					out.Append(gateDec.Reason, "blockedReason")
					continue
				}

				if err := model.desk.Exit(symbol, exitPath, virtual); err != nil {
					errnie.Error(err)
				}
			}
		}
	}

	return out
}

func tokensFromSpell(spell []string, cut tokenizer.Token) tokenizer.Tokens {
	tokens := tokenizer.NewTokens()
	for i, segment := range spell {
		b := 0.0
		if i == len(spell)-1 {
			b = cut.Brightness
		}
		tokens = tokens.Append(tokenizer.NewToken([]string{segment}, b, cut.Meta))
	}
	return tokens
}

func (model *Model) completeExit(
	out *data.Value, symbol string, position *broker.Position,
) {
	if position == nil || position.PnLPct == nil {
		return
	}

	returnFraction := position.RealizedPct

	if returnFraction == 0 {
		returnFraction = position.PnLPct.Float64() / 100.0
	}

	if position.EntryPath != "" {
		model.tree.Load().UpdateResistance(position.EntryPath, ActionEnter, returnFraction)
	}

	if position.HoldPath != "" {
		model.tree.Load().UpdateResistance(position.HoldPath, ActionHold, returnFraction)
	}

	if position.ExitPath != "" && position.ExitPath != position.EntryPath {
		model.tree.Load().UpdateResistance(position.ExitPath, ActionExit, returnFraction)
	}

	result := GateResult{ReturnFraction: returnFraction}
	trigger := position.ExitTrigger
	result.StoppedOut = trigger != "" && (strings.Contains(trigger, "invalidation") || strings.Contains(trigger, "evaporation") || strings.Contains(trigger, "expired") || strings.Contains(trigger, "expiry"))

	if result.StoppedOut {
		result.StoppedOut = returnFraction < 0
	}

	model.gate.Update(result)

	out.Append("exit submitted", "outcome")

	if model.stage != nil {
		model.stage.ResetSequencer(symbol)
	}
}
