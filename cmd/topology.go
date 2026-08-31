package cmd

import (
	"github.com/theapemachine/symm/logic/category"
	"github.com/theapemachine/symm/logic/graph"
	nmruntime "github.com/theapemachine/symm/nomagique/runtime"
	"github.com/theapemachine/symm/system"
	"github.com/theapemachine/symm/types"
)

/*
semanticCore returns the shared stateful semantic stage list mounted on every
producer Workload that delivers at least one of its declared inputs: the
advisory layer, the Influence Graph solver, and the Category solver. Because
each processor holds the AUTHORITATIVE resident state (the advisors' Number
registries keyed per symbol, the graph's lock-free coordinate/edge store, and
the category solver's per-symbol evidence snapshot), mounting the SAME
instances — never a fresh copy per Workload — is what lets trade (CVD/Hawkes)
and Level3 (DepthFlow/Morphology) measurements reach the same semantic state
as ticker measurements.

The label prefix distinguishes each ring in the boundary trace; it has no
effect on correctness. The order matters: advisors fold measurements into
their composed state first, then graph folds them into the coordinate store,
then category re-classifies against the whole per-symbol snapshot.
*/
func semanticCore(
	prefix string,
	advisors advisorNode,
	graphSolver *graph.Solver,
	categorySolver *category.Solver,
) [][]nmruntime.Node[*types.Envelope] {
	return [][]nmruntime.Node[*types.Envelope]{
		{advisors},
		{system.NewTraced(prefix + ".graph", graphSolver)},
		{categorySolver},
	}
}

/*
semanticCoreLight returns the shared semantic state stages without the
Influence Graph solver. It is mounted on the trade, level3, and futures
workloads: those streams produce measurements at market-event frequency, and
running the graph's OLS lag sweeps (~35 candidate pairs × up to 30 lags)
directly in their consumer loop stalls the Disruptor ring. Their measurements
still reach the shared coordinate store through the ticker workload's graph
pass — the same shared *graph.Solver instance — on the ticker cadence, where
relation fitting and MCTS belong. Advisors and the category solver stay mounted
so the composed per-symbol state and evidence still advance in stream order.
*/
func semanticCoreLight(
	prefix string,
	advisors advisorNode,
	categorySolver *category.Solver,
) [][]nmruntime.Node[*types.Envelope] {
	return [][]nmruntime.Node[*types.Envelope]{
		{advisors},
		{categorySolver},
	}
}
