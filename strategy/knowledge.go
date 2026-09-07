package strategy

import (
	"math"
	"time"

	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/hindsight"
	"github.com/theapemachine/symm/nomagique/learning"
	"github.com/theapemachine/symm/types"
)

/* knowledgeMemory preserves the existing model's exponential retention window. */
const knowledgeMemory = 2048.0

/* KnowledgeReading exposes alternative specificity levels and economic sufficient statistics. */
type KnowledgeReading struct {
	Scope    string                `json:"scope"`
	Global   learning.PriorReading `json:"global"`
	Symbol   learning.PriorReading `json:"symbol"`
	Selected learning.PriorReading `json:"selected"`
	Economic EconomicReading       `json:"economic"`
}

/* Prior converts an EconomicReading to PriorReading for backwards-compatible projection. */
func (reading EconomicReading) Prior() learning.PriorReading {
	return learning.PriorReading{
		Depth:             reading.Depth,
		ContextLength:     reading.ContextLength,
		Pending:           reading.Pending,
		Samples:           reading.Samples,
		Defined:           reading.Defined,
		Mean:              reading.Rate,
		Variance:          reading.GrowthVariance,
		VarianceDefined:   reading.VarianceDefined,
		Support:           reading.Support,
		Maturity:          reading.Maturity,
		EvidenceAuthority: reading.EvidenceAuthority,
		Authority:         reading.Authority,
		Memory:            reading.Memory,
	}
}

/* Knowledge owns continuous shared/symbol economic evidence, attribution, and reconstruction. */
type Knowledge struct {
	Model       *EconomicModel
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
	TargetUnavailable    int `json:"targetUnavailable"`
}

/* NewKnowledge shares outcomes across global and symbol evidence. */
func NewKnowledge(grid *learning.Grid) *Knowledge {
	return &Knowledge{
		Model: NewEconomicModel(knowledgeMemory),
		grid:  grid,
	}
}

/*
Reading backs off across scopes using the same evidence semantics as context depth.
A symbol must define dispersion when global evidence does and retain at least the
broader reading's maturity-weighted retained input authority. Specificity wins ties.
*/
func (knowledge *Knowledge) Reading(
	symbol string, accountState string, context []uint64, action LearningAction,
) KnowledgeReading {
	if accountState == "" {
		accountState = "flat"
	}

	globalEconomic := knowledge.Model.Recall([2]string{"", accountState}, context, action)
	symbolEconomic := knowledge.Model.Recall([2]string{symbol, accountState}, context, action)

	reading := KnowledgeReading{
		Scope:    "global",
		Global:   globalEconomic.Prior(),
		Symbol:   symbolEconomic.Prior(),
		Economic: globalEconomic,
	}
	reading.Selected = reading.Global

	local, global := symbolEconomic, globalEconomic

	if local.Defined && (local.VarianceDefined || !global.VarianceDefined) &&
		local.EvidenceAuthority*local.Maturity >= global.EvidenceAuthority*global.Maturity {
		reading.Selected, reading.Scope = reading.Symbol, "symbol"
		reading.Economic = symbolEconomic
	}

	return reading
}

/* recall supplies the selected scope and its economic reading to the selector. */
func (knowledge *Knowledge) recall(
	key [2]string, context []uint64, action LearningAction,
) EconomicReading {
	reading := knowledge.Reading(key[0], key[1], context, action)
	return reading.Economic
}

/* Select uses exploratory/non-exploratory selection with hierarchical economic recall. */
func (knowledge *Knowledge) Select(
	symbol string, accountState string, context []uint64, actions []LearningAction, explore bool,
) (LearningAction, KnowledgeReading, error) {
	if accountState == "" {
		accountState = "flat"
	}

	action, _, err := knowledge.Model.Select(
		[2]string{symbol, accountState}, context, actions, explore, knowledge.recall,
	)

	if err != nil {
		return action, KnowledgeReading{}, err
	}

	return action, knowledge.Reading(symbol, accountState, context, action), nil
}

/* Issue binds both scopes to one ticket and one immutable observation authority. */
func (knowledge *Knowledge) Issue(
	symbol string, accountState string, context []uint64, action LearningAction, authority float64,
) (uint64, error) {
	if symbol == "" {
		return 0, errnie.Err(errnie.Validation, "knowledge: symbol is required", nil)
	}

	if accountState == "" {
		accountState = "flat"
	}

	return knowledge.Model.Issue(
		[2]string{symbol, accountState}, context, action, authority, [2]string{"", accountState},
	)
}

/* Resolve incorporates separate wealth growth and elapsed time into the economic model. */
func (knowledge *Knowledge) Resolve(
	experience learningExperience,
	wealthBefore float64,
	wealthAfter float64,
	elapsed time.Duration,
) (EconomicReading, error) {
	growth := 0.0

	if wealthBefore > 0 && wealthAfter > 0 {
		growth = math.Log(wealthAfter / wealthBefore)
	}

	reading, err := knowledge.Model.Resolve(experience.id, growth, elapsed)

	if err != nil {
		return reading, err
	}

	err = knowledge.attribution.observe(
		experience.tokens[:experience.count], experience.action.Kind, growth, experience.authority,
	)

	return reading, err
}

/*
Warmup replays complete run-scoped issue/resolve pairs. Issue-time context and
quality are authoritative. Historical records without recorded precursor history
cannot fabricate it; they train only the unconditioned action prior.
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

		for regionCount < len(context) && context[regionCount] != 0 && context[regionCount] != FrameDelimiter {
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
			context[index] = learning.RemapCondition(
				context[index], uint64(knowledge.grid.Column(quantity[0], quantity[1])+1),
			)
		}

		action := LearningAction{Kind: types.Action(origin.Action), Power: origin.Power, Reduce: origin.Reduce}

		if action.Kind == "" {
			return report, errnie.Err(errnie.Validation, "knowledge: historical issue action is missing", nil)
		}

		if event.TargetUnit != "" && event.TargetUnit != "return_per_second" && event.TargetUnit != "absolute_return_per_second" {
			return report, errnie.Err(errnie.Validation, "knowledge: unsupported historical target unit", nil)
		}

		if event.TargetUnit != "absolute_return_per_second" && event.AbsoluteSkillTarget == nil {
			report.TargetUnavailable++
			continue
		}

		elapsed := event.At.Sub(origin.At).Seconds()

		if elapsed <= 0 {
			if event.Horizon > 0 {
				elapsed = event.Horizon.Seconds()
			}
		}

		if elapsed <= 0 {
			return report, errnie.Err(errnie.Validation, "knowledge: legacy target requires positive issue-to-resolution time", nil)
		}

		growth := event.Target * elapsed

		if event.AbsoluteSkillTarget != nil {
			growth = *event.AbsoluteSkillTarget
		}

		accountState := "flat"

		if origin.Inventory != "" && origin.Inventory != "0" {
			accountState = "holding"
		}

		if err := knowledge.Model.Observe(
			[2]string{event.Symbol, accountState}, context, action, growth, elapsed, origin.Authority, [2]string{"", accountState},
		); err != nil {
			return report, err
		}

		if len(context) > 0 {
			tokens := make([]uint64, regionCount)

			for index, quantity := range origin.Quantities {
				tokens[index] = uint64(knowledge.grid.Column(quantity[0], quantity[1]) + 1)
			}

			if err := knowledge.attribution.observe(tokens, action.Kind, growth, origin.Authority); err != nil {
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

/* RetainedExperiences is the Kish limit of the geometric retention weights. */
func (knowledge *Knowledge) RetainedExperiences() int {
	decay := 1 - 1/knowledgeMemory
	return int(math.Ceil((1 + decay) / (1 - decay)))
}
