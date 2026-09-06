package strategy

import (
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/nomagique/learning"
)

/* CapitalReading keeps source and symbol specificity alternatives separate. */
type CapitalReading struct {
	Source   string                `json:"source"`
	Scope    string                `json:"scope"`
	Virtual  KnowledgeReading      `json:"virtual"`
	Actual   KnowledgeReading      `json:"actual"`
	Selected learning.PriorReading `json:"selected"`
}

/*
CapitalKnowledge transfers allocation evidence between symbols while retaining
separate virtual and actual evidence. One chosen reading supplies every sample,
support and authority value; correlated teachers are never pooled.
*/
type CapitalKnowledge struct {
	Model *learning.Model[[2]string, CapitalAction]
}

/* NewCapitalKnowledge shares an aging clock, not observations, across evidence levels. */
func NewCapitalKnowledge() *CapitalKnowledge {
	return &CapitalKnowledge{Model: learning.NewModel[[2]string, CapitalAction](knowledgeMemory)}
}

/* Reading chooses actual specificity only when its retained evidence supports that choice. */
func (knowledge *CapitalKnowledge) Reading(context []uint64, action CapitalAction) CapitalReading {
	reading := CapitalReading{
		Source:  "capital_virtual",
		Virtual: knowledge.scope("capital_virtual", context, action),
		Actual:  knowledge.scope("capital_account", context, action),
	}
	reading.Selected, reading.Scope = reading.Virtual.Selected, reading.Virtual.Scope
	actual, virtual := reading.Actual.Selected, reading.Virtual.Selected

	if actual.Defined && (actual.VarianceDefined || !virtual.VarianceDefined) &&
		actual.EvidenceAuthority*actual.Maturity >= virtual.EvidenceAuthority*virtual.Maturity {
		reading.Selected, reading.Source, reading.Scope = actual, "capital_account", reading.Actual.Scope
	}

	return reading
}

/* scope recalls alternative global and symbol beliefs within exactly one teacher source. */
func (knowledge *CapitalKnowledge) scope(source string, context []uint64, action CapitalAction) KnowledgeReading {
	symbol := action.Symbol
	action.Symbol = ""
	reading := KnowledgeReading{Scope: "global",
		Global: knowledge.Model.Recall([2]string{source, ""}, context, action),
		Symbol: knowledge.Model.Recall([2]string{source, symbol}, context, action)}
	reading.Selected = reading.Global
	local, global := reading.Symbol, reading.Global

	if symbol != "" && local.Defined && (local.VarianceDefined || !global.VarianceDefined) &&
		local.EvidenceAuthority*local.Maturity >= global.EvidenceAuthority*global.Maturity {
		reading.Selected, reading.Scope = local, "symbol"
	}

	return reading
}

/* Issue binds symbol and transferable allocation evidence under one source and ticket. */
func (knowledge *CapitalKnowledge) Issue(source string, context []uint64, action CapitalAction, authority float64) (uint64, error) {
	if source != "capital_virtual" && source != "capital_account" {
		return 0, errnie.Err(errnie.Validation, "capital knowledge: observed teacher source required", nil)
	}

	symbol := action.Symbol
	action.Symbol = ""

	if symbol == "" {
		return knowledge.Model.Issue([2]string{source, ""}, context, action, authority)
	}

	return knowledge.Model.Issue([2]string{source, symbol}, context, action, authority, [2]string{source, ""})
}

/* Observe restores complete source-labelled experiences without inventing another observation. */
func (knowledge *CapitalKnowledge) Observe(source string, context []uint64, action CapitalAction, target, authority float64) error {
	if source != "capital_virtual" && source != "capital_account" {
		return errnie.Err(errnie.Validation, "capital knowledge: historical teacher source required", nil)
	}

	symbol := action.Symbol
	action.Symbol = ""

	if symbol == "" {
		return knowledge.Model.Observe([2]string{source, ""}, context, action, target, authority)
	}

	return knowledge.Model.Observe([2]string{source, symbol}, context, action, target, authority, [2]string{source, ""})
}
