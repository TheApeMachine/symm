import { useSelector } from "@tanstack/react-store";
import { useEffect, useState } from "react";
import { signals } from "#/collections/app";
import { metricOf, metricsWithPrefix } from "#/components/ui";
import { hubBaseUrl } from "#/lib/hub";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { RadixTreeViz } from "./radix-tree-viz";
import type { CognitionTreeResponse, SymbolProgress } from "./types";

/*
tokensOf rebuilds the token strings ("R01-R02") of a Training Measurement
from its path_token:IDX:PART Metrics, the same form the stored keys use.
*/
export const tokensOf = (measurement?: MeasurementT): string[] => {
	const tokens: string[][] = [];
	for (const [key, region] of metricsWithPrefix(measurement, "path_token:")) {
		const [idx, part] = key.split(":").map(Number);
		tokens[idx] ??= [];
		tokens[idx][part] = `R${String(region).padStart(2, "0")}`;
	}
	return tokens.filter(Boolean).map((parts) => parts.filter(Boolean).join("-"));
};

/* keysHash is FNV-1a 32 over the keys joined by newlines, as strategy.keysHash. */
export const keysHash = (keys: string[]): number => {
	let hash = 2166136261;
	const text = keys.join("\n");
	for (let i = 0; i < text.length; i++) {
		hash = Math.imul(hash ^ text.charCodeAt(i), 16777619) >>> 0;
	}
	return hash >>> 0;
};

/* heldProgress: the token path of every symbol the desk holds, as last sent. */
const heldProgress = (state: Record<string, { toArray(): MeasurementT[] }>) => {
	const out: Record<string, SymbolProgress> = {};
	for (const [symbol, ring] of Object.entries(state)) {
		const all = ring?.toArray() ?? [];
		const last = all[all.length - 1];
		if (metricOf(last, "position_open") !== 1) continue;
		const withTokens = [...all].reverse().find((m) => metricOf(m, "token_depth") !== undefined);
		const tokens = tokensOf(withTokens);
		if (tokens.length === 0) continue;
		const indices = [...metricsWithPrefix(withTokens, "match_key:").values()];
		const hash = metricOf(withTokens, "stored_paths_hash");
		out[symbol] = { symbol, path: tokens.join("/"), depth: tokens.length, tokens, indices, hash } as SymbolProgress & { indices: number[]; hash?: number };
	}
	return JSON.stringify(out);
};

/*
CognitiveTree draws the stored token paths the /cognition/tree endpoint
reports, with the held symbols' live token paths overlaid.
*/
export const CognitiveTree = () => {
	const [tree, setTree] = useState<CognitionTreeResponse | null>(null);
	const [failed, setFailed] = useState(false);
	const symbols = JSON.parse(useSelector(signals.training, heldProgress)) as Record<string, SymbolProgress>;

	useEffect(() => {
		let alive = true;
		const load = async () => {
			try {
				const res = await fetch(`${hubBaseUrl()}/cognition/tree`);
				if (!res.ok) throw new Error(String(res.status));
				const data = (await res.json()) as CognitionTreeResponse;
				if (alive) {
					setTree(data);
					setFailed(false);
				}
			} catch {
				if (alive) setFailed(true);
			}
		};
		load();
		const timer = window.setInterval(load, 10000);
		return () => {
			alive = false;
			window.clearInterval(timer);
		};
	}, []);

	if (!tree || (tree.keys ?? []).length === 0) {
		return (
			<div className="flex-1 flex flex-col items-center justify-center gap-2 text-(--f3)">
				<div className="w-16 h-16 rounded-full border-2 border-dashed border-(--line)" />
				<span className="text-sm">
					{!tree ? (failed ? "Cannot reach the tree endpoint" : "Loading stored paths…") : "No stored token paths yet"}
				</span>
				{tree && <span className="text-[10px] text-(--f4)">run detect and Train to store paths</span>}
			</div>
		);
	}

	const keys = tree.keys ?? [];
	const treeHash = keysHash(keys);
	const resolved: Record<string, SymbolProgress> = {};
	for (const [symbol, raw] of Object.entries(symbols)) {
		const { indices, hash, ...progress } = raw as SymbolProgress & { indices: number[]; hash?: number };
		// Indices only mean something against the key list Step matched on.
		const matched =
			hash === treeHash
				? indices.map((idx) => keys[idx]).filter(Boolean).map((key) => key.slice(0, key.lastIndexOf("/")))
				: undefined;
		resolved[symbol] = { ...progress, matched };
	}

	return <RadixTreeViz keys={keys} data={tree.root} symbols={resolved} feasible={tree.feasible ?? []} />;
};
