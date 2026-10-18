import { useSelector } from "@tanstack/react-store";
import { useEffect } from "react";
import { type CognitionMatch, cognitionStore } from "#/collections/cognition";
import { sendCommand } from "#/providers/websocket";
import { ModelProblem } from "./model-problem";
import { RadixTreeViz } from "./radix-tree-viz";
import type { SymbolProgress } from "./types";

export const progressOf = (match: CognitionMatch): SymbolProgress => ({
	symbol: match.symbol,
	path: match.tokens.join("/"),
	depth: match.stored ? match.depth : match.tokens.length,
	tokens: match.tokens,
	matched:
		match.stored && match.action ? [`${match.match}/${match.action}.bin`] : [],
	rejected: match.rejected ?? [],
});

export const CognitiveTree = () => {
	const epoch = useSelector(cognitionStore, (state) => state.epoch);
	const policyKeys = useSelector(cognitionStore, (state) =>
		state.policyKeys.length > 0 ? state.policyKeys : (state.tree?.keys ?? []),
	);
	const matches = useSelector(cognitionStore, (state) => state.matches);
	const trading = useSelector(
		cognitionStore,
		(state): boolean | undefined => state.tree?.trading,
	);
	const gate = useSelector(cognitionStore, (state) => state.tree?.gate ?? null);
	const tradingLabel =
		trading === true
			? "paper trading"
			: trading === false
				? "not trading"
				: "trading unknown";

	useEffect(() => {
		try {
			sendCommand({ type: "s3", prefix: "policy/current" });
		} catch (err) {
			console.error(err);
		}
	}, []);

	if (policyKeys.length === 0) {
		return (
			<div className="flex-1 flex flex-col min-h-0">
				<ModelProblem />
				<div
					className="flex-1 flex flex-col items-center justify-center gap-2 text-(--f3)"
					data-testid="cognitive-tree-empty"
				>
					<div className="w-16 h-16 rounded-full border-2 border-dashed border-(--line)" />
					<span className="text-sm">
						{epoch !== "0"
							? "paper trading inactive: no trained paths"
							: "waiting for policy/current"}
					</span>
				</div>
			</div>
		);
	}

	const symbols: Record<string, SymbolProgress> = {};

	for (const [symbol, match] of Object.entries(matches)) {
		symbols[symbol] = progressOf(match);
	}

	return (
		<div className="flex-1 flex flex-col min-h-0">
			<div
				className="flex gap-3 px-2 py-1 text-[10px] text-(--f3) border-b border-(--line)"
				data-testid="cognitive-tree-meta"
			>
				<span>policy epoch {epoch}</span>
				<span>{policyKeys.length} paths</span>
				<span>{tradingLabel}</span>
				<span>
					{gate === null
						? "live tokens from train/regions on tick"
						: gate.enabled
							? `gated z≥${gate.surpriseZ} · warm-up ${gate.warmup} · trained on ${gate.tickEpoch}`
							: `ungated · trained on ${gate.tickEpoch}`}
				</span>
			</div>
			<RadixTreeViz keys={policyKeys} symbols={symbols} feasible={[]} />
		</div>
	);
};
