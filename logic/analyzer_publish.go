package logic

import (
	"time"

	"github.com/theapemachine/datura"
	"github.com/theapemachine/errnie"
	"github.com/theapemachine/symm/audit"
	"github.com/theapemachine/symm/logic/manifold"
	"github.com/theapemachine/symm/types"
)

/*
publishMeasured emits manifold, resonance, causal, and hypothesis frames.
Manifold states go one symbol per UI message so the websocket/worker never
clones a 100+ row lattice payload in a single DRAW. Shared Sensorium grids
still ride only the first carrier row; the terminal inherits them onto focus.
*/
func (analyzer *Analyzer) publishMeasured(
	thesis *types.Thesis,
	states []manifold.State,
) {
	publishStarted := time.Now()

	for _, state := range wireManifold(states) {
		analyzer.publish(datura.Map[any]{
			"manifold": []manifold.State{state},
		})
	}

	if len(thesis.Resonance) > 0 {
		analyzer.publish(datura.Map[any]{"resonance": thesis.Resonance})
	}

	if len(thesis.Causal) > 0 {
		analyzer.publish(datura.Map[any]{"causal": thesis.Causal})
	}

	if len(thesis.Hypotheses) > 0 {
		analyzer.publish(datura.Map[any]{"hypotheses": thesis.Hypotheses})
	}

	errnie.Error(audit.Phase(analyzer.recorder, thesis.Tick, "publish", map[string]any{
		"ns":       time.Since(publishStarted).Nanoseconds(),
		"manifold": len(states),
	}))
}

/*
wireManifold keeps shared Sensorium lattices on one carrier row and strips
per-symbol particle clouds elsewhere. Symbol rows stay small so they can be
published individually without OOM; the pilot-wave field is the shared lattice.
*/
func wireManifold(states []manifold.State) []manifold.State {
	wired := make([]manifold.State, len(states))
	fieldKept := false

	for index, state := range states {
		wired[index] = state
		wired[index].Particles = nil

		if len(state.Rho) == 0 {
			continue
		}

		if fieldKept {
			wired[index].Rho = nil
			wired[index].PsiMag2 = nil
			wired[index].GuidanceVelX = nil
			wired[index].GuidanceVelZ = nil
			wired[index].Wave = nil
			continue
		}

		fieldKept = true
		wired[index].Particles = state.Particles
	}

	return wired
}

/*
publishCognition emits cognition and forecast frames after REM consolidation,
and projects ready winners onto thesis.Categories so the terminal category rail
is not an empty shell while classifications live only on Cognition.
*/
func (analyzer *Analyzer) publishCognition(thesis *types.Thesis) {
	cognition := make([]types.Cognition, 0, 8)

	thesis.Cognition.Range(func(key, value any) bool {
		reading, ok := value.(types.Cognition)

		if ok {
			cognition = append(cognition, reading)
		}

		return true
	})

	thesis.Categories = nil

	if len(cognition) > 0 {
		analyzer.publish(datura.Map[any]{"cognition": cognition})
		analyzer.projectCategories(thesis, cognition)
	}

	if len(thesis.Forecasts) > 0 {
		analyzer.publish(datura.Map[any]{"forecasts": thesis.Forecasts})
	}

	if len(thesis.Categories) > 0 {
		analyzer.publish(datura.Map[any]{"categories": thesis.Categories})
	}
}

/*
projectCategories writes one Category row per ready cognition winner so strategy
publish and the thesis modal share the same classification surface.
*/
func (analyzer *Analyzer) projectCategories(
	thesis *types.Thesis,
	cognition []types.Cognition,
) {
	if thesis == nil {
		return
	}

	for _, reading := range cognition {
		if !reading.Ready || reading.Winner == "" || reading.Symbol == "" {
			continue
		}

		category := types.Category{
			Symbol:     reading.Symbol,
			Type:       types.CategoryType(reading.Winner),
			Confidence: reading.Confidence,
			Surprisal:  reading.EntropyBits,
			Strength:   reading.LookaheadScore,
			Maturity:   float64(reading.Cohort),
		}

		analyzer.attachCategoryEvidence(thesis, &category)
		thesis.Categories = append(thesis.Categories, category)
	}
}

/*
attachCategoryEvidence reads the symbol's composed evidence graph and fills the
category's Supporting and Opposing lists with the active directional phenomena
bearing on the winner. Winner rows are cognition attractors (buy/sell/balanced),
not graph category nodes, so the evidence is the graph's long-stance reading:
straight for a buy winner, mirrored for a sell winner, absent for balanced —
never a fabricated proof for a label the graph does not model.
*/
func (analyzer *Analyzer) attachCategoryEvidence(
	thesis *types.Thesis,
	category *types.Category,
) {
	if thesis == nil || thesis.Graphs == nil || category.Symbol == "" {
		return
	}

	value, found := thesis.Graphs.Load(category.Symbol)

	if !found {
		return
	}

	evidenceGraph, ok := value.(*types.Graph)

	if !ok || evidenceGraph == nil {
		return
	}

	evidence := evidenceGraph.LongEntryEvidence()

	switch category.Type {
	case "buy":
		category.Supporting, category.Opposing = evidence.Favors, evidence.Opposes
	case "sell":
		category.Supporting, category.Opposing = evidence.Opposes, evidence.Favors
	}
}

/*
publishGraphs emits composed evidence graphs so the thesis modal can render
measurement relationships for the focused symbol.
*/
func (analyzer *Analyzer) publishGraphs(thesis *types.Thesis) {
	if thesis == nil || thesis.Graphs == nil {
		return
	}

	graphs := make([]*types.Graph, 0)

	thesis.Graphs.Range(func(_, value any) bool {
		evidenceGraph, ok := value.(*types.Graph)

		if !ok || evidenceGraph == nil || len(evidenceGraph.Nodes()) == 0 {
			return true
		}

		graphs = append(graphs, evidenceGraph)

		return true
	})

	if len(graphs) == 0 {
		return
	}

	analyzer.publish(datura.Map[any]{"graphs": graphs})
}
