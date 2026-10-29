package strategy

import (
	"context"
	"strings"
	"sync/atomic"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/nomagique/data"
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
	decision := model.tree.Load().Predict(&tokens, coder)
	out.Append(decision.Action.String(), "action")

	// 6. Execute based on the decision
	switch decision.Action {
	case ActionEnter:
		out.Append(true, "enter")

		// Physical & environmental checks for ENTERS
		gateDec := model.gate.EvaluateEnter(symbol, observed, coder)

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

		// Physical & environmental checks for EXITS
		for _, virtual := range []bool{false, true} {
			if pos := model.desk.Position(symbol, virtual); pos != nil {
				gateDec := model.gate.EvaluateExit(symbol, observed, coder, pos)

				if !gateDec.Allowed {
					out.Append(true, "blocked")
					out.Append(gateDec.Reason, "blockedReason")
					continue
				}

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

	// Realized fractional return (e.g. +0.05 for +5% win, -0.02 for -2% loss).
	// RealizedPct is the fee-net return captured at exit from the exit VWAP; it
	// is what the round trip actually made. PnLPct is a fallback only for a lot
	// that settled with no realized capture (a venue-flat close).
	returnFraction := position.RealizedPct

	if returnFraction == 0 {
		returnFraction = position.PnLPct.Float64() / 100.0
	}

	// 1. Train Path Tree resistance
	if position.EntryPath != "" {
		model.tree.Load().UpdateResistance(position.EntryPath, ActionEnter, returnFraction)
	}

	if position.ExitPath != "" && position.ExitPath != position.EntryPath {
		model.tree.Load().UpdateResistance(position.ExitPath, ActionExit, returnFraction)
	}

	// 2. Train Gate environmental hurdles. StoppedOut drives the rejection
	// direction: a stop-out (invalidation/evaporation) means "take profits
	// sooner" (lower rejection); a harvested exit means "hold longer".
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
