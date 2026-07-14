import {
	clearCanvas,
	drawGrid,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import type { GraphFrame, GraphNodeWire } from "#/types/thesis";

export type GraphNodePosition = {
	x: number;
	y: number;
};

const LABEL_LIMIT = 24;
const RING_COUNT = 6;

const layoutCache = new Map<string, Map<string, GraphNodePosition>>();

const stableHash = (key: string): number => {
	let hash = 2166136261;

	for (let index = 0; index < key.length; index += 1) {
		hash ^= key.charCodeAt(index);
		hash = Math.imul(hash, 16777619);
	}

	return hash >>> 0;
};

const stableUnit = (key: string): number => stableHash(key) / 0xffffffff;

/*
graphTopologyKey fingerprints node and edge identity so layout and canvas
repaints ignore timestamp-only graph frame churn from live thesis ticks.
*/
export const graphTopologyKey = (graph: GraphFrame | null): string => {
	if (graph === null) {
		return "";
	}

	const nodes = graph.nodes
		.map((node) => node.key)
		.sort()
		.join("\0");
	const edges = graph.edges
		.map((edge) => `${edge.from}\t${edge.to}\t${edge.type}`)
		.sort()
		.join("\0");

	return `${graph.symbol}\n${nodes}\n${edges}`;
};

const layoutCacheKey = (
	graph: GraphFrame,
	width: number,
	height: number,
): string =>
	`${graphTopologyKey(graph)}:${Math.round(width)}:${Math.round(height)}`;

const edgeTone = (type: string): string => {
	switch (type) {
		case "supports":
			return TERMINAL_COLORS.green;
		case "contradicts":
			return TERMINAL_COLORS.red;
		case "conditions":
			return TERMINAL_COLORS.cyan;
		case "leads":
			return TERMINAL_COLORS.amber;
		case "lags":
			return TERMINAL_COLORS.muted;
		case "stale":
			return "#c49a4d";
		default:
			return TERMINAL_COLORS.lineStrong;
	}
};

const nodeLabel = (node: GraphNodeWire): string => {
	const measurement = node.measurement;
	const source =
		typeof measurement.source === "string" ? measurement.source : "source";
	const metric =
		typeof measurement.metric === "string" ? measurement.metric : "metric";

	return `${source}/${metric}`.slice(0, 28);
};

/*
layoutEvidenceGraph assigns each measurement node a stable polar coordinate from
its key alone so live thesis ingest cannot re-shuffle the entire graph every tick.
*/
export const layoutEvidenceGraph = (
	graph: GraphFrame,
	width: number,
	height: number,
): Map<string, GraphNodePosition> => {
	// ponytail: hash-derived ring/angle placement is a naive stand-in for a force-
	// directed layout, and the fixed 48-entry FIFO cache is not LRU; upgrade both
	// when interactive graph editing or cross-session layout stability is required.
	const cacheKey = layoutCacheKey(graph, width, height);
	const cached = layoutCache.get(cacheKey);

	if (cached !== undefined) {
		return cached;
	}

	const positions = new Map<string, GraphNodePosition>();
	const pad = 42;
	const centerX = width / 2;
	const centerY = height / 2;
	const maxRadius = Math.max(48, Math.min(width, height) * 0.38 - pad);

	for (const node of graph.nodes) {
		const unit = stableUnit(node.key);
		const ring = Math.floor(unit * RING_COUNT) % RING_COUNT;
		const radius = pad + ((ring + 1) / (RING_COUNT + 1)) * maxRadius;
		const angle = stableUnit(`${node.key}:angle`) * Math.PI * 2;

		positions.set(node.key, {
			x: centerX + Math.cos(angle) * radius,
			y: centerY + Math.sin(angle) * radius,
		});
	}

	layoutCache.set(cacheKey, positions);

	if (layoutCache.size > 48) {
		const first = layoutCache.keys().next().value;

		if (first !== undefined) {
			layoutCache.delete(first);
		}
	}

	return positions;
};

/*
drawEvidenceGraph renders one symbol-local measurement relationship graph.
*/
export const drawEvidenceGraph = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	graph: GraphFrame | null,
): void => {
	clearCanvas(context, width, height);
	drawGrid(context, width, height, 24);

	if (graph === null || graph.nodes.length === 0) {
		context.fillStyle = TERMINAL_COLORS.muted;
		context.font = "11px JetBrains Mono, monospace";
		context.fillText("waiting for evidence graph frames", 18, height * 0.48);

		return;
	}

	const positions = layoutEvidenceGraph(graph, width, height);
	const showLabels = graph.nodes.length <= LABEL_LIMIT;

	for (const edge of graph.edges) {
		const from = positions.get(edge.from);
		const to = positions.get(edge.to);

		if (from === undefined || to === undefined) {
			continue;
		}

		context.strokeStyle = edgeTone(edge.type);
		context.globalAlpha = 0.72;
		context.lineWidth =
			edge.type === "supports" || edge.type === "contradicts" ? 1.8 : 1.2;
		context.beginPath();
		context.moveTo(from.x, from.y);
		context.lineTo(to.x, to.y);
		context.stroke();
		context.globalAlpha = 1;
	}

	for (const node of graph.nodes) {
		const position = positions.get(node.key);

		if (position === undefined) {
			continue;
		}

		context.fillStyle = TERMINAL_COLORS.amber;
		context.shadowBlur = 8;
		context.shadowColor = TERMINAL_COLORS.amber;
		context.beginPath();
		context.arc(position.x, position.y, 5, 0, Math.PI * 2);
		context.fill();
		context.shadowBlur = 0;

		if (showLabels) {
			context.fillStyle = TERMINAL_COLORS.foreground;
			context.font = "9px JetBrains Mono, monospace";
			context.fillText(nodeLabel(node), position.x + 8, position.y + 3);
		}
	}

	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "10px JetBrains Mono, monospace";
	context.fillText(
		`${graph.nodes.length} nodes · ${graph.edges.length} edges · ${graph.at.slice(11, 19)}${showLabels ? "" : " · labels hidden"}`,
		18,
		height - 14,
	);
};
