import {
	clamp01,
	clearCanvas,
	drawGrid,
	TERMINAL_COLORS,
} from "#/components/terminal/canvas";
import type {
	GraphEdgeWire,
	GraphFrame,
	GraphNodeKind,
	GraphNodeWire,
} from "#/types/thesis";

export type GraphNodePosition = {
	x: number;
	y: number;
};

/*
GraphHit is one interactive target the canvas resolves under the pointer so the
rail can render an inspection tooltip for a node or an edge.
*/
export type GraphNodeHit = {
	kind: "node";
	node: GraphNodeWire;
	position: GraphNodePosition;
};

export type GraphEdgeHit = {
	kind: "edge";
	edge: GraphEdgeWire;
	midpoint: GraphNodePosition;
};

export type GraphHit = GraphNodeHit | GraphEdgeHit;

/*
GraphScene is the resolved layout for one frame: node positions plus the hit
targets, so drawing and pointer resolution share one computation per frame.
*/
export type GraphScene = {
	positions: Map<string, GraphNodePosition>;
	nodes: Map<string, GraphNodeWire>;
	width: number;
	height: number;
};

const NODE_HIT_RADIUS = 11;
const EDGE_HIT_DISTANCE = 6;

const stableHash = (key: string): number => {
	let hash = 2166136261;

	for (let index = 0; index < key.length; index += 1) {
		hash ^= key.charCodeAt(index);
		hash = Math.imul(hash, 16777619);
	}

	return hash >>> 0;
};

const stableUnit = (key: string): number => stableHash(key) / 0xffffffff;

const measurementString = (
	measurement: Record<string, unknown>,
	field: string,
): string => {
	const value = measurement[field];

	return typeof value === "string" ? value : "";
};

const measurementNumber = (
	measurement: Record<string, unknown>,
	field: string,
): number | null => {
	const value = measurement[field];

	return typeof value === "number" && Number.isFinite(value) ? value : null;
};

/*
nodeKind resolves the node's role, preferring the explicit wire kind and falling
back to the descriptive source so older frames still classify correctly.
*/
export const nodeKind = (node: GraphNodeWire): GraphNodeKind => {
	if (node.kind === "category" || node.kind === "concept") {
		return node.kind;
	}

	if (node.kind === "measurement") {
		return "measurement";
	}

	const source = measurementString(node.measurement, "source");

	if (source === "category") {
		return "category";
	}

	if (source === "causal") {
		return "concept";
	}

	return "measurement";
};

const isHypothesis = (node: GraphNodeWire): boolean =>
	nodeKind(node) !== "measurement";

/*
nodeIdentity is a node's stable semantic identity across ticks, independent of
the per-tick MeasurementKey (which embeds At.UnixNano and so changes every tick).
Layout seeds off this so depthflow/loaded_score keeps its slot each frame instead
of teleporting. Category/concept nodes are already tick-stable by their key.
*/
export const nodeIdentity = (node: GraphNodeWire): string => {
	const kind = nodeKind(node);

	if (kind === "category") {
		return `category/${node.category ?? measurementString(node.measurement, "metric")}`;
	}

	if (kind === "concept") {
		return `concept/${measurementString(node.measurement, "metric")}`;
	}

	const measurement = node.measurement;

	return [
		measurementString(measurement, "source"),
		measurementString(measurement, "metric"),
		measurementString(measurement, "subject"),
		measurementString(measurement, "side"),
		measurementString(measurement, "peer"),
	].join("/");
};

/*
nodeLabel is the on-canvas text: category/concept nodes read as their subject,
measurements as source/metric.
*/
export const nodeLabel = (node: GraphNodeWire): string => {
	const measurement = node.measurement;

	if (nodeKind(node) === "category") {
		return node.category ?? measurementString(measurement, "metric") ?? "category";
	}

	if (nodeKind(node) === "concept") {
		return measurementString(measurement, "metric") || "concept";
	}

	const source = measurementString(measurement, "source") || "source";
	const metric = measurementString(measurement, "metric") || "metric";

	return `${source}/${metric}`;
};

/*
normalizedMagnitude maps a measurement's normalized reading onto 0..1 for value-
driven fill intensity. Hypothesis nodes report full intensity.
*/
const normalizedMagnitude = (node: GraphNodeWire): number => {
	if (isHypothesis(node)) {
		return 1;
	}

	const normalized = measurementNumber(node.measurement, "normalized");

	if (normalized === null) {
		return 0.25;
	}

	return clamp01(Math.abs(normalized));
};

const validityState = (node: GraphNodeWire): string =>
	measurementString(node.measurement, "validity") ||
	(typeof node.measurement.validity === "object" &&
	node.measurement.validity !== null
		? measurementString(
				node.measurement.validity as Record<string, unknown>,
				"state",
			)
		: "");

const isDegraded = (node: GraphNodeWire): boolean => {
	const state = validityState(node);

	return state === "provisional" || state === "invalid";
};

/*
edgeTone colors an edge by its relationship so support, conflict, conditioning,
and lead-lag are distinguishable at a glance.
*/
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
		case "redundant":
			return TERMINAL_COLORS.lineStrong;
		case "stale":
			return "#c49a4d";
		default:
			return TERMINAL_COLORS.lineStrong;
	}
};

const DIRECTED_EDGES = new Set(["leads", "lags", "conditions"]);
const EMPHASIZED_EDGES = new Set(["supports", "contradicts", "conditions"]);

/*
The edge-type legend rendered on the canvas so the color coding is self-
describing rather than tribal knowledge.
*/
const LEGEND: Array<{ type: string; label: string }> = [
	{ type: "supports", label: "supports" },
	{ type: "contradicts", label: "contradicts" },
	{ type: "conditions", label: "conditions →" },
	{ type: "leads", label: "leads →" },
	{ type: "lags", label: "lags →" },
];

type NodeGeometry = {
	node: GraphNodeWire;
	hub: boolean;
	degree: number;
};

/*
buildGeometry indexes nodes, marks hypothesis hubs, and counts each node's
degree so layout can place high-degree hubs centrally and size nodes by reach.
*/
const buildGeometry = (graph: GraphFrame): Map<string, NodeGeometry> => {
	const geometry = new Map<string, NodeGeometry>();

	for (const node of graph.nodes) {
		geometry.set(node.key, {
			node,
			hub: isHypothesis(node),
			degree: 0,
		});
	}

	for (const edge of graph.edges) {
		const from = geometry.get(edge.from);
		const to = geometry.get(edge.to);

		if (from !== undefined) {
			from.degree += 1;
		}

		if (to !== undefined) {
			to.degree += 1;
		}
	}

	return geometry;
};

/*
layoutEvidenceGraph places hypothesis hubs on an inner ring ordered by degree and
pulls each measurement toward the centroid of the hubs it connects to, so the
"which signals support this category" structure is spatially legible. Nodes with
no hub attachment fall onto a stable outer ring. Layout is deterministic (seeded
by node key) so it does not reshuffle on live ticks.
*/
export const layoutEvidenceGraph = (
	graph: GraphFrame,
	width: number,
	height: number,
): Map<string, GraphNodePosition> => {
	const positions = new Map<string, GraphNodePosition>();
	const geometry = buildGeometry(graph);
	const centerX = width / 2;
	const centerY = height / 2;
	const pad = 54;
	const maxRadius = Math.max(60, Math.min(width, height) * 0.42 - pad);

	// Hubs are ordered by stable identity (not volatile degree) so their ring
	// order does not reshuffle as edges appear and disappear each tick.
	const hubs = [...geometry.values()]
		.filter((entry) => entry.hub)
		.sort((left, right) =>
			nodeIdentity(left.node) < nodeIdentity(right.node) ? -1 : 1,
		);

	const baseHubRadius = hubs.length <= 1 ? 0 : maxRadius * 0.34;

	// Each hub's angle and a small radial offset are seeded purely from its own
	// identity, so it keeps the same slot regardless of which other hubs are
	// present this tick. There is no index/count term — any dependence on the
	// live hub set would move every hub when one appears or drops. The identity-
	// seeded radial jitter separates hubs that hash to nearby angles so they do
	// not stack on top of each other.
	for (const entry of hubs) {
		const identity = nodeIdentity(entry.node);
		const angle = stableUnit(`${identity}:hub`) * Math.PI * 2;
		const radius =
			baseHubRadius === 0
				? 0
				: baseHubRadius * (0.78 + stableUnit(`${identity}:hubr`) * 0.44);

		positions.set(entry.node.key, {
			x: centerX + Math.cos(angle) * radius,
			y: centerY + Math.sin(angle) * radius,
		});
	}

	// Measurements gravitate to the mean of their connected hubs; unattached
	// nodes take a stable outer-ring slot so nothing overlaps the hub cluster.
	const hubOf = new Map<string, string[]>();

	for (const edge of graph.edges) {
		const fromHub = geometry.get(edge.from)?.hub === true;
		const toHub = geometry.get(edge.to)?.hub === true;

		if (fromHub && !toHub) {
			(hubOf.get(edge.to) ?? hubOf.set(edge.to, []).get(edge.to))?.push(
				edge.from,
			);
		}

		if (toHub && !fromHub) {
			(hubOf.get(edge.from) ?? hubOf.set(edge.from, []).get(edge.from))?.push(
				edge.to,
			);
		}
	}

	for (const entry of geometry.values()) {
		if (entry.hub) {
			continue;
		}

		const identity = nodeIdentity(entry.node);
		const attachedHubs = hubOf.get(entry.node.key) ?? [];
		const spread = stableUnit(identity);

		if (attachedHubs.length === 0) {
			const angle = stableUnit(`${identity}:angle`) * Math.PI * 2;
			const radius = maxRadius * (0.82 + spread * 0.16);

			positions.set(entry.node.key, {
				x: centerX + Math.cos(angle) * radius,
				y: centerY + Math.sin(angle) * radius,
			});

			continue;
		}

		let sumX = 0;
		let sumY = 0;
		let count = 0;

		for (const hubKey of attachedHubs) {
			const hubPosition = positions.get(hubKey);

			if (hubPosition !== undefined) {
				sumX += hubPosition.x;
				sumY += hubPosition.y;
				count += 1;
			}
		}

		const anchorX = count > 0 ? sumX / count : centerX;
		const anchorY = count > 0 ? sumY / count : centerY;
		const outward = Math.atan2(anchorY - centerY, anchorX - centerX);
		const jitter = (spread - 0.5) * 0.9;
		const radius = baseHubRadius + maxRadius * (0.34 + spread * 0.28);

		positions.set(entry.node.key, {
			x: anchorX + Math.cos(outward + jitter) * radius * 0.5,
			y: anchorY + Math.sin(outward + jitter) * radius * 0.5,
		});
	}

	return positions;
};

/*
graphVisualKey fingerprints topology (node keys + typed edges) so the canvas only
repaints when structure changes, not on timestamp-only churn.
*/
export const graphVisualKey = (graph: GraphFrame | null): string => {
	if (graph === null) {
		return "";
	}

	const nodes = graph.nodes
		.map((node) => `${node.key}:${nodeKind(node)}`)
		.sort()
		.join("\0");
	const edges = graph.edges
		.map((edge) => `${edge.from}\t${edge.to}\t${edge.type}`)
		.sort()
		.join("\0");

	return `${graph.symbol}\n${nodes}\n${edges}`;
};

const clampToBounds = (
	position: GraphNodePosition,
	width: number,
	height: number,
): GraphNodePosition => ({
	x: Math.min(width - 12, Math.max(12, position.x)),
	y: Math.min(height - 12, Math.max(12, position.y)),
});

/*
buildScene resolves positions once so drawing and pointer hit-testing agree.
*/
export const buildScene = (
	graph: GraphFrame,
	width: number,
	height: number,
): GraphScene => {
	const raw = layoutEvidenceGraph(graph, width, height);
	const positions = new Map<string, GraphNodePosition>();
	const nodes = new Map<string, GraphNodeWire>();

	for (const node of graph.nodes) {
		const position = raw.get(node.key);

		if (position !== undefined) {
			positions.set(node.key, clampToBounds(position, width, height));
			nodes.set(node.key, node);
		}
	}

	return { positions, nodes, width, height };
};

const nodeRadius = (geometryDegree: number, hub: boolean): number => {
	if (hub) {
		return 7 + Math.min(6, geometryDegree * 0.9);
	}

	return 4.5;
};

/*
drawArrowhead paints a filled triangle at the target end, aimed along `approach`
(the direction the line arrives from — the chord for a straight edge, or the
control-point→target tangent for a curved one) so the head sits flush with the
line whether or not it is bent.
*/
const drawArrowhead = (
	context: CanvasRenderingContext2D,
	approach: GraphNodePosition,
	to: GraphNodePosition,
	color: string,
	targetRadius: number,
): void => {
	const angle = Math.atan2(to.y - approach.y, to.x - approach.x);
	const tipX = to.x - Math.cos(angle) * (targetRadius + 2);
	const tipY = to.y - Math.sin(angle) * (targetRadius + 2);
	const size = 7;

	context.fillStyle = color;
	context.beginPath();
	context.moveTo(tipX, tipY);
	context.lineTo(
		tipX - Math.cos(angle - 0.4) * size,
		tipY - Math.sin(angle - 0.4) * size,
	);
	context.lineTo(
		tipX - Math.cos(angle + 0.4) * size,
		tipY - Math.sin(angle + 0.4) * size,
	);
	context.closePath();
	context.fill();
};

/*
reciprocalPairs returns the set of unordered node-pair keys that carry more than
one edge between the same two nodes (in either direction) — e.g. an anchor that
Leads a follower while the follower Lags it. Those edges are bowed apart so they
do not draw on top of each other.
*/
export const reciprocalPairs = (graph: GraphFrame): Set<string> => {
	const counts = new Map<string, number>();

	for (const edge of graph.edges) {
		if (edge.from === edge.to) {
			continue;
		}

		const key = pairKey(edge.from, edge.to);
		counts.set(key, (counts.get(key) ?? 0) + 1);
	}

	const pairs = new Set<string>();

	for (const [key, count] of counts) {
		if (count > 1) {
			pairs.add(key);
		}
	}

	return pairs;
};

export const pairKey = (a: string, b: string): string =>
	a < b ? `${a}\0${b}` : `${b}\0${a}`;

/*
edgeControlPoint returns the quadratic-Bézier control point that bows an edge to
one side of the straight chord. The offset direction is fixed by the edge's own
orientation (from<to vs from>to), so the two edges of a reciprocal pair curve to
opposite sides deterministically. Straight edges return the midpoint.
*/
export const edgeControlPoint = (
	edge: GraphEdgeWire,
	from: GraphNodePosition,
	to: GraphNodePosition,
	bend: boolean,
): GraphNodePosition => {
	const midX = (from.x + to.x) / 2;
	const midY = (from.y + to.y) / 2;

	if (!bend) {
		return { x: midX, y: midY };
	}

	// The chord direction is canonicalized to the smaller→larger endpoint so it
	// is identical for both edges of a reciprocal pair; the side is then keyed to
	// this edge's own orientation. Without canonicalization the reversed chord
	// and reversed side cancel, bowing both edges to the same visual side.
	const forward = edge.from < edge.to;
	const dx = (forward ? to.x - from.x : from.x - to.x);
	const dy = (forward ? to.y - from.y : from.y - to.y);
	const length = Math.hypot(dx, dy) || 1;
	const side = forward ? 1 : -1;
	const offset = Math.min(46, length * 0.22) * side;

	// Perpendicular to the canonical chord.
	return {
		x: midX + (-dy / length) * offset,
		y: midY + (dx / length) * offset,
	};
};

const mixHex = (from: string, to: string, ratio: number): string => {
	const clamped = clamp01(ratio);
	const parse = (hex: string) => [
		Number.parseInt(hex.slice(1, 3), 16),
		Number.parseInt(hex.slice(3, 5), 16),
		Number.parseInt(hex.slice(5, 7), 16),
	];
	const [fr, fg, fb] = parse(from);
	const [tr, tg, tb] = parse(to);
	const channel = (a: number, b: number) =>
		Math.round(a + (b - a) * clamped)
			.toString(16)
			.padStart(2, "0");

	return `#${channel(fr, tr)}${channel(fg, tg)}${channel(fb, tb)}`;
};

const kindFill = (node: GraphNodeWire): string => {
	const kind = nodeKind(node);

	if (kind === "category") {
		return TERMINAL_COLORS.cyan;
	}

	if (kind === "concept") {
		return TERMINAL_COLORS.amber;
	}

	// Measurement fill intensity tracks normalized magnitude.
	return mixHex(
		TERMINAL_COLORS.surface,
		TERMINAL_COLORS.foreground,
		0.25 + normalizedMagnitude(node) * 0.75,
	);
};

const drawLegend = (
	context: CanvasRenderingContext2D,
	width: number,
): void => {
	const startX = width - 132;
	let y = 16;

	context.font = "9px JetBrains Mono, monospace";
	context.textAlign = "left";

	for (const item of LEGEND) {
		context.strokeStyle = edgeTone(item.type);
		context.globalAlpha = 0.9;
		context.lineWidth = 2;
		context.beginPath();
		context.moveTo(startX, y);
		context.lineTo(startX + 16, y);
		context.stroke();
		context.globalAlpha = 1;

		context.fillStyle = TERMINAL_COLORS.muted;
		context.fillText(item.label, startX + 22, y + 3);
		y += 14;
	}
};

/*
drawEvidenceGraph renders one symbol-local evidence graph: hypothesis hubs, their
supporting/opposing measurements, directed lead-lag/conditions edges, a value-
driven node encoding, and a legend. The optional hover key highlights a node and
its incident edges for focused inspection.
*/
export const drawEvidenceGraph = (
	context: CanvasRenderingContext2D,
	width: number,
	height: number,
	graph: GraphFrame | null,
	scene?: GraphScene | null,
	hoverKey?: string | null,
): void => {
	clearCanvas(context, width, height);
	drawGrid(context, width, height, 24);

	if (graph === null || graph.nodes.length === 0) {
		context.fillStyle = TERMINAL_COLORS.muted;
		context.font = "11px JetBrains Mono, monospace";
		context.textAlign = "left";
		context.fillText("waiting for evidence graph frames", 18, height * 0.48);

		return;
	}

	const resolved = scene ?? buildScene(graph, width, height);
	const positions = resolved.positions;
	const geometry = buildGeometry(graph);
	const reciprocal = reciprocalPairs(graph);

	const incident = new Set<string>();

	if (hoverKey) {
		for (const edge of graph.edges) {
			if (edge.from === hoverKey || edge.to === hoverKey) {
				incident.add(`${edge.from}\t${edge.to}\t${edge.type}`);
			}
		}
	}

	for (const edge of graph.edges) {
		const from = positions.get(edge.from);
		const to = positions.get(edge.to);

		if (from === undefined || to === undefined) {
			continue;
		}

		const edgeId = `${edge.from}\t${edge.to}\t${edge.type}`;
		const highlighted = hoverKey ? incident.has(edgeId) : false;
		const dimmed = hoverKey !== null && hoverKey !== undefined && !highlighted;
		const bend = reciprocal.has(pairKey(edge.from, edge.to));
		const control = edgeControlPoint(edge, from, to, bend);

		context.strokeStyle = edgeTone(edge.type);
		context.globalAlpha = dimmed ? 0.12 : highlighted ? 0.95 : 0.62;
		context.lineWidth = EMPHASIZED_EDGES.has(edge.type)
			? highlighted
				? 2.4
				: 1.8
			: 1.1;
		context.beginPath();
		context.moveTo(from.x, from.y);

		if (bend) {
			context.quadraticCurveTo(control.x, control.y, to.x, to.y);
		} else {
			context.lineTo(to.x, to.y);
		}

		context.stroke();

		if (DIRECTED_EDGES.has(edge.type) && !dimmed) {
			const targetHub = geometry.get(edge.to)?.hub === true;
			// A curved edge arrives along the control-point tangent; a straight
			// one along the chord from its origin.
			drawArrowhead(
				context,
				bend ? control : from,
				to,
				edgeTone(edge.type),
				nodeRadius(geometry.get(edge.to)?.degree ?? 0, targetHub),
			);
		}

		context.globalAlpha = 1;
	}

	const showLabels = graph.nodes.length <= 40;

	for (const node of graph.nodes) {
		const position = positions.get(node.key);

		if (position === undefined) {
			continue;
		}

		const entry = geometry.get(node.key);
		const hub = entry?.hub ?? false;
		const radius = nodeRadius(entry?.degree ?? 0, hub);
		const focused = hoverKey === node.key;
		const faded =
			hoverKey !== null && hoverKey !== undefined && !focused
				? !isNeighbor(graph, hoverKey, node.key)
				: false;

		context.globalAlpha = faded ? 0.28 : 1;

		const fill = kindFill(node);
		const degraded = isDegraded(node);

		context.beginPath();
		context.fillStyle = fill;

		if (focused) {
			context.shadowBlur = 14;
			context.shadowColor = fill;
		} else if (hub) {
			context.shadowBlur = 8;
			context.shadowColor = fill;
		}

		if (hub) {
			// Hypothesis nodes are squares (category) or diamonds (concept).
			if (nodeKind(node) === "concept") {
				drawDiamond(context, position, radius + 1);
			} else {
				drawSquare(context, position, radius);
			}
		} else {
			context.arc(position.x, position.y, radius, 0, Math.PI * 2);
			context.fill();
		}

		context.shadowBlur = 0;

		if (degraded) {
			context.strokeStyle = TERMINAL_COLORS.red;
			context.setLineDash([2, 2]);
			context.lineWidth = 1;
			context.beginPath();
			context.arc(position.x, position.y, radius + 2, 0, Math.PI * 2);
			context.stroke();
			context.setLineDash([]);
		}

		if (showLabels || hub || focused) {
			context.fillStyle = focused
				? TERMINAL_COLORS.foreground
				: hub
					? TERMINAL_COLORS.foreground
					: TERMINAL_COLORS.muted;
			context.font = hub
				? "10px JetBrains Mono, monospace"
				: "9px JetBrains Mono, monospace";
			context.textAlign = "left";
			context.fillText(
				nodeLabel(node).slice(0, 30),
				position.x + radius + 4,
				position.y + 3,
			);
		}

		context.globalAlpha = 1;
	}

	drawLegend(context, width);

	context.fillStyle = TERMINAL_COLORS.muted;
	context.font = "10px JetBrains Mono, monospace";
	context.textAlign = "left";
	context.fillText(
		`${graph.nodes.length} nodes · ${graph.edges.length} edges · ${graph.at.slice(11, 19)}`,
		18,
		height - 14,
	);
};

const drawSquare = (
	context: CanvasRenderingContext2D,
	position: GraphNodePosition,
	radius: number,
): void => {
	const size = radius * 1.7;
	context.fillRect(position.x - size / 2, position.y - size / 2, size, size);
};

const drawDiamond = (
	context: CanvasRenderingContext2D,
	position: GraphNodePosition,
	radius: number,
): void => {
	context.beginPath();
	context.moveTo(position.x, position.y - radius);
	context.lineTo(position.x + radius, position.y);
	context.lineTo(position.x, position.y + radius);
	context.lineTo(position.x - radius, position.y);
	context.closePath();
	context.fill();
};

const isNeighbor = (
	graph: GraphFrame,
	anchorKey: string,
	candidateKey: string,
): boolean => {
	if (anchorKey === candidateKey) {
		return true;
	}

	for (const edge of graph.edges) {
		if (
			(edge.from === anchorKey && edge.to === candidateKey) ||
			(edge.to === anchorKey && edge.from === candidateKey)
		) {
			return true;
		}
	}

	return false;
};

const distanceToSegment = (
	point: GraphNodePosition,
	start: GraphNodePosition,
	end: GraphNodePosition,
): number => {
	const dx = end.x - start.x;
	const dy = end.y - start.y;
	const lengthSquared = dx * dx + dy * dy;

	if (lengthSquared === 0) {
		return Math.hypot(point.x - start.x, point.y - start.y);
	}

	const t = clamp01(
		((point.x - start.x) * dx + (point.y - start.y) * dy) / lengthSquared,
	);
	const projX = start.x + t * dx;
	const projY = start.y + t * dy;

	return Math.hypot(point.x - projX, point.y - projY);
};

/*
distanceToQuadratic approximates the distance from a point to a quadratic Bézier
by sampling it into short segments — enough for hover tolerance on a gently bowed
edge without a closed-form root solve.
*/
const distanceToQuadratic = (
	point: GraphNodePosition,
	start: GraphNodePosition,
	control: GraphNodePosition,
	end: GraphNodePosition,
): number => {
	const samples = 12;
	let previous = start;
	let best = Number.POSITIVE_INFINITY;

	for (let step = 1; step <= samples; step += 1) {
		const t = step / samples;
		const inverse = 1 - t;
		const current = {
			x:
				inverse * inverse * start.x +
				2 * inverse * t * control.x +
				t * t * end.x,
			y:
				inverse * inverse * start.y +
				2 * inverse * t * control.y +
				t * t * end.y,
		};

		best = Math.min(best, distanceToSegment(point, previous, current));
		previous = current;
	}

	return best;
};

/*
hitTest resolves what sits under the pointer: a node first (they take priority),
otherwise the nearest edge within tolerance, so hover inspection can show the
right detail.
*/
export const hitTest = (
	graph: GraphFrame,
	scene: GraphScene,
	x: number,
	y: number,
): GraphHit | null => {
	let closestNode: GraphNodeHit | null = null;
	let closestNodeDistance = NODE_HIT_RADIUS;

	for (const node of graph.nodes) {
		const position = scene.positions.get(node.key);

		if (position === undefined) {
			continue;
		}

		const distance = Math.hypot(position.x - x, position.y - y);

		if (distance <= closestNodeDistance) {
			closestNodeDistance = distance;
			closestNode = { kind: "node", node, position };
		}
	}

	if (closestNode !== null) {
		return closestNode;
	}

	let closestEdge: GraphEdgeHit | null = null;
	let closestEdgeDistance = EDGE_HIT_DISTANCE;
	const reciprocal = reciprocalPairs(graph);

	for (const edge of graph.edges) {
		const from = scene.positions.get(edge.from);
		const to = scene.positions.get(edge.to);

		if (from === undefined || to === undefined) {
			continue;
		}

		const bend = reciprocal.has(pairKey(edge.from, edge.to));
		const control = edgeControlPoint(edge, from, to, bend);
		const distance = bend
			? distanceToQuadratic({ x, y }, from, control, to)
			: distanceToSegment({ x, y }, from, to);

		if (distance <= closestEdgeDistance) {
			closestEdgeDistance = distance;
			closestEdge = { kind: "edge", edge, midpoint: control };
		}
	}

	return closestEdge;
};
