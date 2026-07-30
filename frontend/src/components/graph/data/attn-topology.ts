import type { AttnRecording, HeadMode } from "@/lib/attention-recordings";

import { Graph } from "../core/graph";

export type LayeredTokenGraph = {
	graph: Graph;
	nodeNameByLayerToken: string[][];
};

function makeNodeName(
	layerIndex: number,
	tokenIndex: number,
	token: string,
): string {
	return `L${layerIndex}:${tokenIndex}:${token}`;
}

function getMeanWeight(
	weightsByHead: number[][][],
	row: number,
	col: number,
): number {
	let sum = 0;
	let used = 0;
	for (const head of weightsByHead) {
		const r = head[row];
		if (!r) continue;
		const w = r[col];
		if (typeof w !== "number") continue;
		sum += w;
		used++;
	}
	if (used === 0) return 0;
	return sum / used;
}

function getHeadWeight(
	weightsByHead: number[][][],
	headIndex: number,
	row: number,
	col: number,
): number {
	const head = weightsByHead[headIndex];
	if (!head) return 0;
	const r = head[row];
	if (!r) return 0;
	const w = r[col];
	if (typeof w !== "number") return 0;
	return w;
}

function pickTopKEdges(params: {
	weightsByHead: number[][][];
	headMode: HeadMode;
	row: number;
	tokenCount: number;
	k: number;
	minWeight: number;
}): Array<{ col: number; w: number }> {
	const { weightsByHead, headMode, row, tokenCount, k, minWeight } = params;

	const scored: Array<{ col: number; w: number }> = [];
	for (let col = 0; col < tokenCount; col++) {
		if (col === row) continue;
		const w =
			headMode.kind === "mean"
				? getMeanWeight(weightsByHead, row, col)
				: getHeadWeight(weightsByHead, headMode.index, row, col);
		if (w > minWeight) scored.push({ col, w });
	}

	scored.sort((a, b) => b.w - a.w);
	return scored.slice(0, Math.max(0, k));
}

export function buildLayeredTokenGraphFromAttention(params: {
	recording: AttnRecording;
	tokens: string[];
	headMode: HeadMode;
	includeVerticalEdges?: boolean;
}): LayeredTokenGraph {
	const { recording, tokens, headMode, includeVerticalEdges = true } = params;

	const layerCount = recording.layers.length;
	if (layerCount === 0) {
		return { graph: new Graph(), nodeNameByLayerToken: [] };
	}

	const matrices0 = recording.layers[0]?.attn?.matrices;
	const tokenCount =
		matrices0?.[0]?.length ?? recording.layers[0]?.act?.values?.length ?? 0;
	if (tokenCount === 0) {
		return { graph: new Graph(), nodeNameByLayerToken: [] };
	}

	// Keep topology readable: attention matrices are often dense (especially causal),
	// so we only connect each query token to its top-k attended tokens.
	const TOP_K_PER_TOKEN = 6;
	const MIN_EDGE_WEIGHT = 1e-3;

	const epochKey = new Graph().settings.epoch;

	const g = new Graph();
	const nodeNameByLayerToken: string[][] = new Array(layerCount);

	for (let layerIndex = 0; layerIndex < layerCount; layerIndex++) {
		const iso = new Date(layerIndex * 1000).toISOString();
		const layerNames: string[] = new Array(tokenCount);

		for (let tokenIndex = 0; tokenIndex < tokenCount; tokenIndex++) {
			const token = tokens[tokenIndex] ?? String(tokenIndex);
			const name = makeNodeName(layerIndex, tokenIndex, token);
			layerNames[tokenIndex] = name;
			g.addNode(name, { [epochKey]: iso });
		}

		nodeNameByLayerToken[layerIndex] = layerNames;
	}

	if (includeVerticalEdges) {
		for (let layerIndex = 0; layerIndex < layerCount - 1; layerIndex++) {
			for (let tokenIndex = 0; tokenIndex < tokenCount; tokenIndex++) {
				const a = nodeNameByLayerToken[layerIndex]?.[tokenIndex];
				const b = nodeNameByLayerToken[layerIndex + 1]?.[tokenIndex];
				if (a && b)
					g.addEdge(a, b, { kind: "vertical", layerIndex, tokenIndex });
			}
		}
	}

	for (let layerIndex = 0; layerIndex < layerCount; layerIndex++) {
		const matrices = recording.layers[layerIndex]?.attn?.matrices;
		if (!matrices || matrices.length === 0) continue;

		for (let row = 0; row < tokenCount; row++) {
			const topEdges = pickTopKEdges({
				weightsByHead: matrices,
				headMode,
				row,
				tokenCount,
				k: TOP_K_PER_TOKEN,
				minWeight: MIN_EDGE_WEIGHT,
			});

			for (const { col, w } of topEdges) {
				const source = nodeNameByLayerToken[layerIndex]?.[row];
				const target = nodeNameByLayerToken[layerIndex]?.[col];
				if (!source || !target) continue;

				g.addEdge(source, target, { kind: "attn", layerIndex, row, col, w });
			}
		}
	}

	return { graph: g, nodeNameByLayerToken };
}
