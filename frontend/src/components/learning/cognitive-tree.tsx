import { useSelector } from "@tanstack/react-store";
import { useEffect, useMemo } from "react";
import { atoms, signals } from "#/collections/app";
import { bookAtom, triePrefix } from "#/collections/book";
import {
	type CognitionMatch,
	cognitionStore,
	ingestExploreListPrefix,
} from "#/collections/cognition";
import type { PathToken, Tokens } from "#/collections/envelope";
import { pathsListingFromEnvelope } from "#/hub/s3-prefix-compact";
import { sendCommand } from "#/providers/websocket";
import { ModelProblem } from "./model-problem";
import { RadixTreeViz } from "./radix-tree-viz";
import type { SymbolProgress } from "./types";

const pathTokens = (cut: Tokens | undefined): string[] => {
	if (!Array.isArray(cut)) return [];

	const out: string[] = [];

	for (const raw of cut) {
		if (!raw || typeof raw !== "object") continue;
		const symbols = (raw as PathToken).symbols;
		if (!Array.isArray(symbols) || symbols.length === 0) continue;
		if (symbols.length === 1) {
			const lone = symbols[0];
			if (lone !== undefined) out.push(lone);
		} else {
			out.push(`[${symbols.join("-")}]`);
		}
	}

	return out;
};

export const progressOf = (match: CognitionMatch): SymbolProgress => {
	const phase =
		match.policyPath ||
		match.triggerPath ||
		match.livePath ||
		match.tokens.join("/");
	const tokens =
		match.tokens.length > 0 ? match.tokens : phase.split("/").filter(Boolean);

	return {
		symbol: match.symbol,
		path: phase,
		depth: match.depth > 0 ? match.depth : tokens.length,
		tokens,
		matched:
			match.stored && match.action
				? [`${match.match}/${match.action}.bin`]
				: [],
		rejected: match.rejected ?? [],
	};
};

export const CognitiveTree = () => {
	const epoch = useSelector(cognitionStore, (state) => state.epoch);
	const policyKeys = useSelector(cognitionStore, (state) => state.policyKeys);
	const thresholds = useSelector(cognitionStore, (state) => state.thresholds);
	const matches = useSelector(cognitionStore, (state) => state.matches);
	const trading = useSelector(
		cognitionStore,
		(state): boolean | undefined => state.tree?.trading,
	);
	const gate = useSelector(cognitionStore, (state) => state.tree?.gate ?? null);
	const book = useSelector(bookAtom, (state) => state);
	const focus = useSelector(atoms.focus, (state) => state);
	const cut = useSelector(signals.paths, (state) => state[focus]);
	const pathbook = useSelector(signals.pathbook, (state) => state);
	const prefix = triePrefix(book);
	const tradingLabel =
		trading === true
			? "paper trading"
			: trading === false
				? "not trading"
				: "trading unknown";

	useEffect(() => {
		const compact = pathsListingFromEnvelope(pathbook);
		if (compact) ingestExploreListPrefix(compact);
	}, [pathbook]);

	useEffect(() => {
		try {
			sendCommand({ type: "s3", prefix });
		} catch (err) {
			console.error(err);
		}
	}, [prefix]);

	const symbols = useMemo(() => {
		const next: Record<string, SymbolProgress> = {};
		for (const [symbol, match] of Object.entries(matches)) {
			next[symbol] = progressOf(match);
		}

		const tokens = pathTokens(cut);
		if (focus !== "" && tokens.length > 0) {
			const phase = tokens.join("/");
			next[focus] = {
				symbol: focus,
				path: phase,
				depth: tokens.length,
				tokens,
				matched: [],
				rejected: [],
			};
		}

		return next;
	}, [matches, focus, cut]);

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
							? `no ${book} paths yet (${prefix}/…/enter.bin, hold.bin, or exit.bin)`
							: `waiting for ${prefix}`}
					</span>
				</div>
			</div>
		);
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
						? "band-local lit · primary cut · exit trigger prefix"
						: gate.enabled
							? `gated z≥${gate.surpriseZ} · warm-up ${gate.warmup} · trained on ${gate.tickEpoch}`
							: `ungated · trained on ${gate.tickEpoch}`}
				</span>
			</div>
			<RadixTreeViz
				keys={policyKeys}
				thresholds={thresholds}
				symbols={symbols}
				feasible={[]}
			/>
		</div>
	);
};
