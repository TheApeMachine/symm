import { useSelector } from "@tanstack/react-store";
import { type CognitionMatch, cognitionStore } from "#/collections/cognition";
import { ModelProblem } from "./model-problem";
import { RadixTreeViz } from "./radix-tree-viz";
import type { SymbolProgress } from "./types";

/*
progressOf lays one symbol's live match out for the tree: its recent tokens,
and the stored key it matched (<path>/<action>.bin) when one did.
*/
export const progressOf = (match: CognitionMatch): SymbolProgress => ({
	symbol: match.symbol,
	path: match.tokens.join("/"),
	depth: match.stored ? match.depth : match.tokens.length,
	tokens: match.tokens,
	matched:
		match.stored && match.action ? [`${match.match}/${match.action}.bin`] : [],
});

/*
CognitiveTree draws the stored token paths of the CognitionTree make run
loaded (the hub's cognition/tree frame), with each symbol's live match
(cognition/match frames) highlighting its active prefix.
*/
export const CognitiveTree = () => {
	const tree = useSelector(cognitionStore, (state) => state.tree);
	const matches = useSelector(cognitionStore, (state) => state.matches);

	if (!tree || tree.keys.length === 0) {
		return (
			<div className="flex-1 flex flex-col min-h-0">
				<ModelProblem />
				<div
					className="flex-1 flex flex-col items-center justify-center gap-2 text-(--f3)"
					data-testid="cognitive-tree-empty"
				>
					<div className="w-16 h-16 rounded-full border-2 border-dashed border-(--line)" />
					<span className="text-sm">
						{tree
							? "paper trading inactive: no trained paths"
							: "waiting for the hub's cognition tree frame"}
					</span>
					{tree && !tree.problem && (
						<span className="text-[10px] text-(--f4)">
							run detect and train, then restart make run
						</span>
					)}
				</div>
			</div>
		);
	}

	const symbols: Record<string, SymbolProgress> = {};

	for (const [symbol, match] of Object.entries(matches)) {
		symbols[symbol] = progressOf(match);
	}

	const gate = tree.gate;

	return (
		<div className="flex-1 flex flex-col min-h-0">
			<div
				className="flex gap-3 px-2 py-1 text-[10px] text-(--f3) border-b border-(--line)"
				data-testid="cognitive-tree-meta"
			>
				<span>policy epoch {tree.epoch}</span>
				<span>{tree.paths} paths</span>
				<span>{tree.trading ? "paper trading" : "not trading"}</span>
				<span>
					{gate === null
						? "no gate tag"
						: gate.enabled
							? `gated z≥${gate.surpriseZ} · warm-up ${gate.warmup} · trained on ${gate.tickEpoch}`
							: `ungated · trained on ${gate.tickEpoch}`}
				</span>
			</div>
			<RadixTreeViz keys={tree.keys} symbols={symbols} feasible={[]} />
		</div>
	);
};
