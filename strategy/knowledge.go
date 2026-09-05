package strategy

import (
	"math"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/* knowledgeMemory preserves the existing model's exponential retention window. */
const knowledgeMemory = 2048.0

/* KnowledgeReading exposes alternative specificity levels, never summed evidence. */
type KnowledgeReading struct {
	Scope    string                `json:"scope"`
	Global   learning.PriorReading `json:"global"`
	Symbol   learning.PriorReading `json:"symbol"`
	Selected learning.PriorReading `json:"selected"`
}

/* Knowledge owns continuous shared/symbol evidence, attribution and reconstruction. */
type Knowledge struct {
	Model       *learning.Model[[2]string, LearningAction]
	grid        *learning.Grid
	attribution attribution
	Warmed      WarmupReading
}

/* WarmupReading reports exactly which historical knowledge could be reconstructed. */
type WarmupReading struct {
	Resolved             int `json:"resolved"`
	Unconditioned        int `json:"unconditioned"`
	Unpaired             int `json:"unpaired"`
	PortfolioUnavailable int `json:"portfolioUnavailable"`
}

/* NewKnowledge shares one resolution clock across both levels of evidence. */
func NewKnowledge(grid *learning.Grid) *Knowledge {
	return &Knowledge{Model: learning.NewModel[[2]string, LearningAction](knowledgeMemory), grid: grid}
}

/*
Reading backs off across scopes using the same evidence semantics as context depth.
A symbol must define dispersion when global evidence does and retain at least the
broader reading's maturity-weighted retained input authority. Specificity wins ties. A single local sample
cannot override variance-defined global evidence; dormant local evidence ages on
the shared resolution clock. Outcome sign does not decide specificity.
*/
func (knowledge *Knowledge) Reading(symbol string, context []uint64, action LearningAction) KnowledgeReading {
	reading := KnowledgeReading{
		Scope:  "global",
		Global: knowledge.Model.Recall([2]string{"", "virtual"}, context, action),
		Symbol: knowledge.Model.Recall([2]string{symbol, "virtual"}, context, action),
	}
	reading.Selected = reading.Global
	local, global := reading.Symbol, reading.Global

	if local.Defined && (local.VarianceDefined || !global.VarianceDefined) &&
		local.EvidenceAuthority*local.Maturity >= global.EvidenceAuthority*global.Maturity {
		reading.Selected, reading.Scope = local, "symbol"
	}
	return reading
}

/* recall supplies the selected scope and its own pending count to the numerical selector. */
func (knowledge *Knowledge) recall(key [2]string, context []uint64, action LearningAction) learning.PriorReading {
	reading := knowledge.Reading(key[0], context, action)
	return reading.Selected
}

/* Select uses the existing exploratory/non-exploratory mechanism with hierarchical recall. */
func (knowledge *Knowledge) Select(symbol string, context []uint64, actions []LearningAction, explore bool) (LearningAction, KnowledgeReading, error) {
	action, _, err := knowledge.Model.Select([2]string{symbol, "virtual"}, context, actions, explore, knowledge.recall)

	if err != nil {
		return action, KnowledgeReading{}, err
	}
	return action, knowledge.Reading(symbol, context, action), nil
}

/* Issue binds both scopes to one ticket and one immutable observation authority. */
func (knowledge *Knowledge) Issue(symbol string, context []uint64, action LearningAction, authority float64) (uint64, error) {
	if symbol == "" {
		return 0, errnie.Err(errnie.Validation, "knowledge: symbol is required", nil)
	}
	return knowledge.Model.Issue([2]string{symbol, "virtual"}, context, action, authority, [2]string{"", "virtual"})
}

/* Resolve trains both levels once and attributes only the original hot quantities. */
func (knowledge *Knowledge) Resolve(experience learningExperience, target float64) (learning.PriorReading, error) {
	reading, err := knowledge.Model.Resolve(experience.id, target)

	if err != nil {
		return reading, err
	}
	err = knowledge.attribution.observe(experience.tokens[:experience.count], experience.action.Kind, target, experience.authority)
	return reading, err
}

/*
Warmup replays complete run-scoped issue/resolve pairs. Issue-time context and
quality are authoritative. Named quantities are interned into the current Grid;
legacy records without that mapping train only the unconditioned action prior.
No historical numeric column is silently interpreted as a current quantity.
Legacy evidence cannot train portfolio allocation because its account inputs
were never recorded. Neither pending actions nor execution authority is restored.
*/
func (knowledge *Knowledge) Warmup(events []hindsight.LearningEvent) (WarmupReading, error) {
	type identity struct {
		run hindsight.RunID
		id  uint64
	}
	issued := make(map[identity]hindsight.LearningEvent)
	report := WarmupReading{}
	for _, event := range events {
		key := identity{event.Run, event.ID}

		if event.Kind == "issued" {
			issued[key] = event
			continue
		}

		if event.Kind != "resolved" {
			continue
		}
		origin, found := issued[key]
		delete(issued, key)

		if !found {
			report.Unpaired++
			continue
		}

		if origin.Symbol != event.Symbol {
			return report, errnie.Err(errnie.Validation, "knowledge: resolved symbol differs from issue", nil)
		}

		if origin.Authority < 0 || origin.Authority > 1 {
			return report, errnie.Err(errnie.Validation, "knowledge: invalid historical issue authority", nil)
		}
		context := append([]uint64(nil), origin.Context...)
		regionCount := 0
		for regionCount < len(context) && context[regionCount] != 0 {
			regionCount++
		}

		if len(origin.Quantities) != regionCount {
			context = nil
			report.Unconditioned++
		}
		for index := range min(regionCount, len(origin.Quantities)) {
			if context == nil {
				break
			}
			quantity := origin.Quantities[index]
			context[index] = uint64(knowledge.grid.Column(quantity[0], quantity[1]) + 1)
		}
		action := LearningAction{Kind: types.Action(origin.Action), Power: origin.Power, Reduce: origin.Reduce}

		if action.Kind == "" {
			return report, errnie.Err(errnie.Validation, "knowledge: historical issue action is missing", nil)
		}

		if event.TargetUnit != "" && event.TargetUnit != "return_per_second" {
			return report, errnie.Err(errnie.Validation, "knowledge: unsupported historical target unit", nil)
		}

		target := event.Target

		if event.TargetUnit == "" {
			elapsed := event.At.Sub(origin.At).Seconds()

			if elapsed <= 0 {
				return report, errnie.Err(errnie.Validation, "knowledge: legacy interval target requires positive issue-to-resolution time", nil)
			}
			target /= elapsed
		}

		if err := knowledge.Model.Observe([2]string{event.Symbol, "virtual"}, context, action, target, origin.Authority, [2]string{"", "virtual"}); err != nil {
			return report, err
		}

		if len(context) > 0 {
			if err := knowledge.attribution.observe(context[:regionCount], action.Kind, target, origin.Authority); err != nil {
				return report, err
			}
		}
		report.Resolved++
		report.PortfolioUnavailable++
	}
	report.Unpaired += len(issued)
	knowledge.Warmed = report
	return report, nil
}

/* RetainedExperiences is the Kish limit of the existing geometric retention weights. */
func (knowledge *Knowledge) RetainedExperiences() int {
	decay := 1 - 1/knowledgeMemory
	return int(math.Ceil((1 + decay) / (1 - decay)))
}
