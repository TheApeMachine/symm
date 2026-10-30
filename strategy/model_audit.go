package strategy

import (
	nmstore "github.com/theapemachine/nomagique/store"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/broker"
	"github.com/theapemachine/symm/strategy/tokenizer"
)

func writeEnterAudit(
	gate *Gate,
	outcome string,
	symbol string,
	pathKey string,
	gateDec GateDecision,
	tree Decision,
	coder CoderReading,
	observed []*nmstore.Region,
	step tokenizer.StepResult,
	cut tokenizer.Token,
	virtual bool,
	enterErr error,
) {
	rec := map[string]any{
		"model":         "enter",
		"symbol":        symbol,
		"path":          pathKey,
		"outcome":       outcome,
		"coder":         coder,
		"gate":          gateDec,
		"frame":         gate.frameForEnter(observed, gateDec, tree),
		"brightness":    regionBrightness(observed),
		"regions":       litKeys(observed),
		"spell":         step.Spell,
		"stepSilence":   string(step.Silence),
		"token":         cut.String(),
		"cutBrightness": cut.Brightness,
		"tree": map[string]any{
			"action":     tree.Action.String(),
			"virtual":    tree.Virtual,
			"impulse":    tree.Impulse,
			"resistance": tree.Resistance,
		},
		"book": bookSnapshot(gate, symbol),
	}

	if gateDec.Reason != "" {
		rec["reason"] = gateDec.Reason
	}
	if virtual || gateDec.Virtual {
		rec["virtual"] = true
	}
	if enterErr != nil {
		rec["err"] = enterErr.Error()
	}

	audit.Record.Write(rec)
}

func writeExitGateAudit(
	gate *Gate,
	symbol string,
	exitPath string,
	gateDec GateDecision,
	tree Decision,
	coder CoderReading,
	observed []*nmstore.Region,
	exitStep tokenizer.StepResult,
	exitCut tokenizer.Token,
	virtual bool,
	pos *broker.Position,
) {
	rec := map[string]any{
		"model":       "exit",
		"symbol":      symbol,
		"path":        exitPath,
		"outcome":     "gate-blocked",
		"virtual":     virtual,
		"reason":      gateDec.Reason,
		"coder":       coder,
		"gate":        gateDec,
		"frame":       gate.frameForExit(observed, gateDec, tree),
		"brightness":  regionBrightness(observed),
		"regions":     litKeys(observed),
		"spell":       exitStep.Spell,
		"stepSilence": string(exitStep.Silence),
		"token":       exitCut.String(),
		"tree": map[string]any{
			"action":     tree.Action.String(),
			"virtual":    tree.Virtual,
			"impulse":    tree.Impulse,
			"resistance": tree.Resistance,
		},
		"book":     bookSnapshot(gate, symbol),
		"position": positionSnapshot(pos),
	}

	audit.Record.Write(rec)
}

func bookSnapshot(gate *Gate, symbol string) map[string]any {
	out := map[string]any{}
	if gate == nil {
		return out
	}

	if gate.desk != nil {
		out["canSize"] = gate.desk.CanSize(symbol)
	}

	if gate.price != nil {
		if spreadPct, ok := gate.price.SpreadPct(symbol); ok && spreadPct != nil {
			out["spreadPct"] = spreadPct.Float64()
		}
		if dragBudget, ok := gate.price.OpenDragBudgetPct(symbol); ok && dragBudget != nil {
			out["openDragBudgetPct"] = dragBudget.Float64()
		}
		if closeNow, ok := gate.price.EntryCloseNowPnLPct(symbol); ok && closeNow != nil {
			out["entryCloseNowPnLPct"] = closeNow.Float64()
		}
	}

	return out
}

func positionSnapshot(pos *broker.Position) map[string]any {
	if pos == nil {
		return nil
	}

	out := map[string]any{
		"entryPath":    pos.EntryPath,
		"holdPath":     pos.HoldPath,
		"exitPath":     pos.ExitPath,
		"openSurprise": pos.OpenSurprise,
	}

	if pos.PnLPct != nil {
		out["pnlPct"] = pos.PnLPct.Float64()
	}
	if pos.PnL != nil {
		out["pnl"] = pos.PnL.Float64()
	}
	if pos.PeakPnLPct != nil {
		out["peakPnlPct"] = pos.PeakPnLPct.Float64()
	}

	return out
}
