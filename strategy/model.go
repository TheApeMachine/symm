package strategy

import (
	"context"
	"strings"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/data"
	"github.com/theapemachine/symm/nomagique/runtime"
	nmstore "github.com/theapemachine/symm/nomagique/store"
	"github.com/theapemachine/symm/store"
	"github.com/theapemachine/symm/strategy/tokenizer"
	"github.com/theapemachine/symm/system"
)

type Model struct {
	*runtime.System
	desk  *broker.Desk
	stage *Stage
	store *store.Client
	tree  *Tree
	gate  *Gate
}

func NewModel(
	ctx context.Context,
	desk *broker.Desk,
	stage *Stage,
) *Model {
	client := store.NewClient(ctx, system.Cfg.Storage.S3.Bucket)

	return &Model{
		System: runtime.NewSystem(ctx, "model"),
		desk:   desk,
		stage:  stage,
		store:  client,
		tree:   NewTree(ctx, "model"),
		gate:   NewGate(ctx, desk, client),
	}
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

	// 1. Process any closed positions waiting for reinforcement learning
	if pending, ok := model.desk.TakePending(symbol); ok {
		model.completeExit(out, symbol, pending)
	}

	// 2. Tokenize tick and extract spell path
	cut := data.Get[tokenizer.Token](
		source, strings.Join([]string{"stage", symbol, "cut"}, "/"),
	)

	if cut.String() == "" {
		return out
	}

	out.Append(cut, "cut")

	step := data.Get[tokenizer.StepResult](
		source, strings.Join([]string{"stage", symbol, "step"}, "/"),
	)

	spell := tokenizer.CollapseConsecutive(step.Spell)
	pathKey := strings.Join(spell, "/")

	if len(spell) < system.Cfg.Learning.MinPathLength {
		return out
	}

	out.Append(pathKey, "path")
	out.Append(pathKey, "policyPath")

	// 3. Keep exit paths current on any open positions
	for _, virtual := range []bool{false, true} {
		if pos := model.desk.Position(symbol, virtual); pos != nil {
			pos.ExitPath = pathKey
		}
	}

	// 4. Build Tokens structure for Tree.Predict (last cut holds brightness)
	tokens := tokenizer.NewTokens()

	for i, s := range spell {
		b := 0.0

		if i == len(spell)-1 {
			b = cut.Brightness
		}

		tokens = tokens.Append(tokenizer.NewToken([]string{s}, b, cut.Meta))
	}

	// 5. Predict action: Live Impulse (Pins * Coder) vs. Accumulated Resistance
	decision := model.tree.Predict(&tokens, coder)
	out.Append(decision.Action.String(), "action")

	// 6. Execute based on the decision
	switch decision.Action {
	case ActionEnter:
		out.Append(true, "enter")

		// Physical & environmental checks
		gateDec := model.gate.Evaluate(symbol, observed, coder)

		if !gateDec.Allowed {
			out.Append(true, "blocked")
			out.Append(gateDec.Reason, "blockedReason")
			return out
		}

		// If either Tree or Gate has doubt -> Virtual sandbox; otherwise -> Paper
		isVirtual := decision.Virtual || gateDec.Virtual

		// Don't re-enter if we already hold this position tier
		if model.desk.Has(symbol, isVirtual) {
			return out
		}

		if err := model.desk.Enter(symbol, pathKey, isVirtual, true); err != nil {
			return out
		}

		if isVirtual {
			out.Append("virtual enter submitted", "outcome")
		} else {
			out.Append("paper enter submitted", "outcome")
		}

		if pos := model.desk.Position(symbol, isVirtual); pos != nil {
			pos.OpenSurprise = coder.Surprise
			pos.ExitPath = pathKey
		}

		if model.stage != nil {
			model.stage.ResetSequencer(symbol)
		}
	case ActionExit:
		out.Append(false, "enter")

		// If an exit is indicated, close open positions for this symbol
		for _, virtual := range []bool{false, true} {
			if model.desk.Has(symbol, virtual) {
				if err := model.desk.Exit(symbol, pathKey, virtual); err != nil {
					errnie.Error(err)
				}
			}
		}
	}

	return out
}

func (model *Model) completeExit(
	out *data.Value, symbol string, position *broker.Position,
) {
	if position == nil || position.PnLPct == nil {
		return
	}

	// Realized fractional return (e.g. +0.05 for +5% win, -0.02 for -2% loss)
	returnFraction := position.PnLPct.Float64() / 100.0

	// 1. Train Path Tree resistance
	if position.EntryPath != "" {
		model.tree.UpdateResistance(position.EntryPath, ActionEnter, returnFraction)
	}
	
	if position.ExitPath != "" && position.ExitPath != position.EntryPath {
		model.tree.UpdateResistance(position.ExitPath, ActionExit, returnFraction)
	}

	// 2. Train Gate environmental hurdles
	model.gate.Update(returnFraction)

	out.Append("exit submitted", "outcome")

	if model.stage != nil {
		model.stage.ResetSequencer(symbol)
	}
}
