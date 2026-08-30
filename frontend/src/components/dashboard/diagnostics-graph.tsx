import { useMemo, useState } from "react";
import type { EdgeStats, NodeStats } from "#/collections/topology";
import { TOPOLOGY_LIVE_WINDOW_NS } from "#/collections/topology";
import { cn } from "@/lib/utils";

/*
DiagnosticsSelection identifies one stage node shown on the wiring graph. The
detail rail keys off the same value.
*/
export type DiagnosticsSelection = { kind: "stage"; name: string };

/*
HEARTBEAT_NS is roughly how often a healthy stage's own gap should be, used
only to size the "slight vs high" latency bands below as fractions of a beat
rather than free-floating magic numbers.
*/
const HEARTBEAT_NS = 250_000_000;

type HealthTone = "healthy" | "slight" | "high";

const edgeHealth = (latencyNs: number | undefined): HealthTone => {
	if (
		latencyNs === undefined ||
		!Number.isFinite(latencyNs) ||
		latencyNs <= 0
	) {
		return "healthy";
	}

	if (latencyNs < HEARTBEAT_NS / 10) {
		return "healthy";
	}

	if (latencyNs < HEARTBEAT_NS) {
		return "slight";
	}

	return "high";
};

const EDGE_HEALTH_STROKE: Record<HealthTone, string> = {
	healthy: "hsl(140 32% 62%)",
	slight: "hsl(38 92% 50%)",
	high: "hsl(0 72% 51%)",
};

/*
Node geometry and grid pitch live in one abstract coordinate space that the
view scales to fit (see DiagnosticsGraph's viewBox). HALF is the node box's
half-extent; the pitches are deliberately larger, and the difference between
them is the channel every edge routes through. Keeping the gap wider than a
node is what stops boxes colliding once the topology grows past a handful of
stages.
*/
export const HALF = { w: 9, h: 5.5 };
const COL_PITCH = 22;
const ROW_PITCH = 17;

/*
MIN_NODE_PX is the smallest a card may render before its type stops being
readable. The view scales the whole diagram so a card meets this, scrolling
when the topology is too big to fit — a card is never squeezed to fit the
viewport, because an unreadable card defeats the diagram's purpose.
*/
const MIN_NODE_PX = { w: 150, h: 92 };

export const formatCount = (count: number): string =>
	new Intl.NumberFormat("en", { notation: "compact" }).format(count);

export const formatNanos = (nanos: number | undefined): string => {
	if (nanos === undefined || !Number.isFinite(nanos) || nanos <= 0) {
		return "—";
	}

	if (nanos < 1_000) {
		return `${nanos.toFixed(0)}ns`;
	}

	if (nanos < 1_000_000) {
		return `${(nanos / 1_000).toFixed(1)}µs`;
	}

	if (nanos < 1_000_000_000) {
		return `${(nanos / 1_000_000).toFixed(2)}ms`;
	}

	return `${(nanos / 1_000_000_000).toFixed(2)}s`;
};

export const formatRate = (avgGapNs: number): string => {
	if (!Number.isFinite(avgGapNs) || avgGapNs <= 0) {
		return "—";
	}

	const perSecond = 1_000_000_000 / avgGapNs;

	if (perSecond < 1) {
		return `${(perSecond * 60).toFixed(1)}/min`;
	}

	return `${perSecond >= 100 ? perSecond.toFixed(0) : perSecond.toFixed(1)}/s`;
};

type Point = { x: number; y: number };
type NodeSide = "top" | "right" | "bottom" | "left";

type Placement = {
	id: string;
	label: string;
	x: number;
	y: number;
};

export type DiagPort = {
	id: string;
	edgeId: string;
	nodeId: string;
	kind: "out" | "in";
	point: Point;
	side: NodeSide;
	latencyNs?: number;
};

type DiagEdge = {
	id: string;
	from: string;
	to: string;
	d: string;
	points: Point[];
	labelPoint: Point;
	stats: EdgeStats;
};

/*
autoLayout derives a Sugiyama-style layered position for every node purely
from the edge list — no coordinate is ever hand-authored. A node's layer is
the length of the longest path reaching it from any source (a node with no
inbound edge); nodes sharing a layer are spread evenly left to right. This is
what makes the graph render correctly no matter what labels root.go's
Diagnostic stages end up using — the topology is discovered, never declared.
*/
const autoLayout = (
	nodeIds: string[],
	edges: EdgeStats[],
): Map<string, Placement> => {
	const outgoing = new Map<string, string[]>();
	const incoming = new Map<string, string[]>();

	for (const edge of edges) {
		outgoing.set(edge.from, [...(outgoing.get(edge.from) ?? []), edge.to]);
		incoming.set(edge.to, [...(incoming.get(edge.to) ?? []), edge.from]);
	}

	const layer = new Map<string, number>();
	const visiting = new Set<string>();

	// Longest-path-from-a-source layering, memoized. A cycle (which a
	// topology should never have, but a stray self-referential hop could
	// produce) breaks recursion rather than looping forever — a node caught
	// mid-cycle just lands in whatever layer it was first visited at.
	const resolveLayer = (id: string): number => {
		const cached = layer.get(id);
		if (cached !== undefined) return cached;
		if (visiting.has(id)) return 0;

		visiting.add(id);
		const parents = incoming.get(id) ?? [];
		const resolved =
			parents.length === 0
				? 0
				: Math.max(...parents.map((parent) => resolveLayer(parent) + 1));
		visiting.delete(id);
		layer.set(id, resolved);

		return resolved;
	};

	for (const id of nodeIds) {
		resolveLayer(id);
	}

	const byLayer = new Map<number, string[]>();

	for (const id of nodeIds) {
		const l = layer.get(id) ?? 0;
		byLayer.set(l, [...(byLayer.get(l) ?? []), id]);
	}

	const placements = new Map<string, Placement>();

	const layerIndices = [...byLayer.keys()].sort((a, b) => a - b);

	// Seed each layer's order deterministically (most-connected first, then
	// alphabetical) so the sweeps below start from a stable arrangement and a
	// node doesn't jitter position as unrelated stages come and go.
	const order = new Map<number, string[]>();

	for (const l of layerIndices) {
		const ids = byLayer.get(l) ?? [];
		order.set(
			l,
			[...ids].sort((a, b) => {
				const degreeA = (outgoing.get(a)?.length ?? 0) + (incoming.get(a)?.length ?? 0);
				const degreeB = (outgoing.get(b)?.length ?? 0) + (incoming.get(b)?.length ?? 0);
				return degreeB - degreeA || a.localeCompare(b);
			}),
		);
	}

	/*
	Barycenter ordering: a node wants to sit at the average position of the
	neighbours it connects to in the adjacent layer, because an edge between
	two nodes that are near-aligned across layers can't cross much. Sweeping
	down then up repeatedly is the standard Sugiyama crossing-reduction pass —
	without it, layer order is arbitrary and edges tangle even though the
	layering itself is correct.
	*/
	const positionsIn = (l: number): Map<string, number> => {
		const ids = order.get(l) ?? [];
		return new Map(ids.map((id, index) => [id, index]));
	};

	const sweep = (from: number, to: number, neighboursOf: Map<string, string[]>) => {
		const reference = positionsIn(from);
		const ids = order.get(to) ?? [];

		const barycenter = new Map<string, number>();

		ids.forEach((id, index) => {
			const neighbours = (neighboursOf.get(id) ?? []).filter((n) => reference.has(n));

			// A node with no neighbour in the reference layer has no opinion —
			// keep it where it is rather than collapsing it to zero, which
			// would drag unrelated nodes to the left edge.
			barycenter.set(
				id,
				neighbours.length === 0
					? index
					: neighbours.reduce((sum, n) => sum + (reference.get(n) ?? 0), 0) / neighbours.length,
			);
		});

		order.set(
			to,
			[...ids].sort((a, b) => (barycenter.get(a) ?? 0) - (barycenter.get(b) ?? 0) || a.localeCompare(b)),
		);
	};

	const crossingsBetween = (upper: number, lower: number): number => {
		const upperPos = positionsIn(upper);
		const lowerPos = positionsIn(lower);
		const spans: { u: number; v: number }[] = [];

		for (const id of order.get(upper) ?? []) {
			for (const target of outgoing.get(id) ?? []) {
				const u = upperPos.get(id);
				const v = lowerPos.get(target);
				if (u === undefined || v === undefined) continue;
				spans.push({ u, v });
			}
		}

		let count = 0;

		// Two edges cross exactly when their endpoints are in opposite order
		// on the two layers.
		for (let i = 0; i < spans.length; i++) {
			for (let j = i + 1; j < spans.length; j++) {
				const a = spans[i];
				const b = spans[j];
				if ((a.u - b.u) * (a.v - b.v) < 0) count++;
			}
		}

		return count;
	};

	const totalCrossings = (): number => {
		let total = 0;
		for (let i = 0; i + 1 < layerIndices.length; i++) {
			total += crossingsBetween(layerIndices[i], layerIndices[i + 1]);
		}
		return total;
	};

	// Keep the best arrangement seen rather than whatever the last sweep
	// produced — barycenter sweeps aren't monotonic and can end worse than
	// they started.
	let best = new Map([...order].map(([l, ids]) => [l, [...ids]] as const));
	let bestScore = totalCrossings();

	for (let pass = 0; pass < 8 && bestScore > 0; pass++) {
		for (let i = 1; i < layerIndices.length; i++) {
			sweep(layerIndices[i - 1], layerIndices[i], incoming);
		}

		for (let i = layerIndices.length - 2; i >= 0; i--) {
			sweep(layerIndices[i + 1], layerIndices[i], outgoing);
		}

		const score = totalCrossings();

		if (score < bestScore) {
			bestScore = score;
			best = new Map([...order].map(([l, ids]) => [l, [...ids]] as const));
		}
	}

	/*
	Place on a grid whose cell is strictly larger than a node box, then report
	the extent so the view can scale to fit. Spacing nodes as a fraction of a
	fixed 0–100 space (the previous approach) shrinks the gap as stages are
	discovered: at 31 stages the per-layer pitch fell below the node's own
	height and boxes simply overlapped. Deriving the space from the content
	instead means a node never collides regardless of how big the topology
	grows, and leaves real channels between layers for edges to route through.
	*/
	const widest = Math.max(1, ...[...best.values()].map((ids) => ids.length));

	for (const [l, ids] of best) {
		const y = (l + 0.5) * ROW_PITCH;

		// Centre each layer against the widest one so the graph reads as a
		// centred column rather than everything jammed to the left.
		const indent = (widest - ids.length) / 2;

		ids.forEach((id, index) => {
			const x = (indent + index + 0.5) * COL_PITCH;
			placements.set(id, { id, label: id, x, y });
		});
	}

	return placements;
};

const sidesFor = (from: Placement, to: Placement): { from: NodeSide; to: NodeSide } => {
	if (Math.abs(to.y - from.y) > 1) {
		return to.y > from.y
			? { from: "bottom", to: "top" }
			: { from: "top", to: "bottom" };
	}

	return to.x > from.x
		? { from: "right", to: "left" }
		: { from: "left", to: "right" };
};

const portPoint = (
	placement: Placement,
	side: NodeSide,
	index: number,
	count: number,
): Point => {
	const horizontal = side === "top" || side === "bottom";
	const extent = horizontal ? HALF.w : HALF.h;
	const usable = Math.max(0, extent - 0.6);
	const offset = count === 1 ? 0 : -usable + (2 * usable * index) / (count - 1);

	if (side === "top") return { x: placement.x + offset, y: placement.y - HALF.h };
	if (side === "bottom") return { x: placement.x + offset, y: placement.y + HALF.h };
	if (side === "left") return { x: placement.x - HALF.w, y: placement.y + offset };
	return { x: placement.x + HALF.w, y: placement.y + offset };
};

// Stub length must exceed the corner radius, or every bend's rounding gets
// clamped by the short run leading into it and the curve never appears.
const STUB = 3.5;

const stubPoint = (point: Point, side: NodeSide): Point => {
	if (side === "top") return { x: point.x, y: point.y - STUB };
	if (side === "bottom") return { x: point.x, y: point.y + STUB };
	if (side === "left") return { x: point.x - STUB, y: point.y };
	return { x: point.x + STUB, y: point.y };
};

// Routes are drawn as hard orthogonal polylines — square corners read as
// circuit wiring, which is what this diagram is.
const pathOf = (points: Point[]): string =>
	points
		.map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(3)} ${point.y.toFixed(3)}`)
		.join(" ");

/*
labelPointOf centres the latency pill on the route's longest straight run.
Taking a fixed midpoint index instead (the previous approach) could land the
label right on a bend or hard against a node, where it overlaps the box it
belongs to; the longest segment is by definition the one with room for it.
*/
const labelPointOf = (points: Point[]): Point => {
	let best = { x: points[0].x, y: points[0].y };
	let bestLength = -1;

	for (let i = 1; i < points.length; i++) {
		const a = points[i - 1];
		const b = points[i];
		const length = Math.hypot(b.x - a.x, b.y - a.y);

		if (length > bestLength) {
			bestLength = length;
			best = { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
		}
	}

	return best;
};

/*
routeEdge draws an orthogonal route between two ports. A single elbow (the
previous approach) bends at the target's coordinate, which drives the run hard
against the receiving node's edge and makes many edges share the same line —
the "weird routing" that shows up as overlapping right angles.

Instead this routes through the channel *between* the two layers: out of the
source, across at a mid-channel line, then into the target. `lane` shifts that
mid-line per edge so parallel runs occupy visibly distinct tracks rather than
stacking on top of each other. A back-edge (target above the source) can't use
the channel at all, so it detours around the side instead of cutting straight
back through the nodes in between.
*/
const CLEARANCE = 1.6;

type Box = { x: number; y: number; w: number; h: number };

const boxOf = (placement: Placement): Box => ({
	x: placement.x,
	y: placement.y,
	w: HALF.w + CLEARANCE,
	h: HALF.h + CLEARANCE,
});

/*
segmentHitsBox tests one axis-aligned run against a node's padded box. Routes
here are always orthogonal, so a cheap interval overlap on each axis is exact
— no general segment/rectangle intersection needed.
*/
const segmentHitsBox = (a: Point, b: Point, box: Box): boolean => {
	const minX = Math.min(a.x, b.x);
	const maxX = Math.max(a.x, b.x);
	const minY = Math.min(a.y, b.y);
	const maxY = Math.max(a.y, b.y);

	return (
		maxX > box.x - box.w &&
		minX < box.x + box.w &&
		maxY > box.y - box.h &&
		minY < box.y + box.h
	);
};

const routeHitCount = (points: Point[], obstacles: Box[]): number => {
	let hits = 0;

	for (let i = 1; i < points.length; i++) {
		for (const box of obstacles) {
			if (segmentHitsBox(points[i - 1], points[i], box)) hits++;
		}
	}

	return hits;
};

/*
routeEdge picks the least-obstructed orthogonal route between two ports rather
than committing to one shape. Every candidate is scored against the node boxes
it would cut through (the previous router had no obstacle awareness at all, so
a wire happily crossed whatever sat between its endpoints), and the cleanest
wins; ties break toward fewer bends, then shorter length.

Candidates, in rough order of preference: a straight shot, a mid-channel
Z-route through the gap between layers, Z-routes hugging either layer, and
finally wide detours around the outside of the node column for edges that
travel backwards or sideways past intervening stages.
*/
const routeEdge = (
	fromPort: Point,
	toPort: Point,
	fromSide: NodeSide,
	toSide: NodeSide,
	lane: number,
	obstacles: Box[],
	bounds: { minX: number; maxX: number },
): Point[] => {
	const start = stubPoint(fromPort, fromSide);
	const end = stubPoint(toPort, toSide);

	const candidates: Point[][] = [];

	// Straight (or near-straight) shot: only viable when the stubs already
	// line up on one axis.
	if (Math.abs(start.x - end.x) < 0.001 || Math.abs(start.y - end.y) < 0.001) {
		candidates.push([fromPort, start, end, toPort]);
	}

	const vertical = fromSide === "top" || fromSide === "bottom";

	if (vertical) {
		// Z-routes crossing the channel at various depths. The lane offset
		// keeps parallel edges on their own tracks.
		for (const t of [0.5, 0.25, 0.75]) {
			const channel = start.y + (end.y - start.y) * t + lane;
			candidates.push([
				fromPort,
				start,
				{ x: start.x, y: channel },
				{ x: end.x, y: channel },
				end,
				toPort,
			]);
		}

		// Detour around the outside of the whole column — the reliable escape
		// for back-edges and long hops that would otherwise plough through
		// every node in between.
		for (const side of [-1, 1]) {
			const outside =
				side < 0 ? bounds.minX - HALF.w - CLEARANCE - 2 - lane : bounds.maxX + HALF.w + CLEARANCE + 2 + lane;

			candidates.push([
				fromPort,
				start,
				{ x: outside, y: start.y },
				{ x: outside, y: end.y },
				end,
				toPort,
			]);
		}
	} else {
		for (const t of [0.5, 0.25, 0.75]) {
			const channel = start.x + (end.x - start.x) * t + lane;
			candidates.push([
				fromPort,
				start,
				{ x: channel, y: start.y },
				{ x: channel, y: end.y },
				end,
				toPort,
			]);
		}

		// Route above or below the row rather than straight along it.
		for (const side of [-1, 1]) {
			const over = start.y + side * (HALF.h + CLEARANCE + 2) + lane;
			candidates.push([
				fromPort,
				start,
				{ x: start.x, y: over },
				{ x: end.x, y: over },
				end,
				toPort,
			]);
		}
	}

	const lengthOf = (points: Point[]) => {
		let total = 0;
		for (let i = 1; i < points.length; i++) {
			total += Math.abs(points[i].x - points[i - 1].x) + Math.abs(points[i].y - points[i - 1].y);
		}
		return total;
	};

	let best = candidates[0];
	let bestScore = Number.POSITIVE_INFINITY;

	for (const candidate of candidates) {
		// Hits dominate: a route that misses every node beats any shorter
		// route that cuts through one.
		const score = routeHitCount(candidate, obstacles) * 1000 + candidate.length * 4 + lengthOf(candidate) * 0.05;

		if (score < bestScore) {
			bestScore = score;
			best = candidate;
		}
	}

	return best;
};

export const buildDiagnosticsGraph = (nodes: Map<string, NodeStats>, edges: Map<string, EdgeStats>) => {
	const nodeIds = Array.from(nodes.keys());
	const edgeList = Array.from(edges.values());
	const placements = autoLayout(nodeIds, edgeList);

	const attachments = new Map<string, { edge: EdgeStats; end: "from" | "to"; opposite: number }[]>();
	const sides = new Map<string, { from: NodeSide; to: NodeSide }>();

	for (const edge of edgeList) {
		const from = placements.get(edge.from);
		const to = placements.get(edge.to);
		if (!from || !to) continue;

		const edgeSides = sidesFor(from, to);
		const id = `${edge.from}>${edge.to}`;
		sides.set(id, edgeSides);

		const fromKey = `${edge.from}:${edgeSides.from}`;
		const toKey = `${edge.to}:${edgeSides.to}`;
		attachments.set(fromKey, [
			...(attachments.get(fromKey) ?? []),
			{ edge, end: "from", opposite: edgeSides.from === "top" || edgeSides.from === "bottom" ? to.x : to.y },
		]);
		attachments.set(toKey, [
			...(attachments.get(toKey) ?? []),
			{ edge, end: "to", opposite: edgeSides.to === "top" || edgeSides.to === "bottom" ? from.x : from.y },
		]);
	}

	const portsMap = new Map<string, Point>();
	const ports: DiagPort[] = [];

	for (const group of attachments.values()) {
		group.sort((a, b) => a.opposite - b.opposite || `${a.edge.from}>${a.edge.to}`.localeCompare(`${b.edge.from}>${b.edge.to}`));

		group.forEach((attachment, index) => {
			const id = `${attachment.edge.from}>${attachment.edge.to}`;
			const side = sides.get(id);
			if (!side) return;

			const placement = placements.get(attachment.end === "from" ? attachment.edge.from : attachment.edge.to);
			if (!placement) return;

			const point = portPoint(placement, attachment.end === "from" ? side.from : side.to, index, group.length);
			portsMap.set(`${id}:${attachment.end}`, point);
			ports.push({
				id: `${id}:${attachment.end}`,
				edgeId: id,
				nodeId: placement.id,
				kind: attachment.end === "from" ? "out" : "in",
				point,
				side: attachment.end === "from" ? side.from : side.to,
				latencyNs: attachment.edge.avgLatencyNs,
			});
		});
	}

	const edgesOut: DiagEdge[] = [];

	/*
	Edges crossing the same channel get distinct lanes. Grouping by the pair of
	layers an edge spans (rounded, since a layer's y is shared by every node in
	it) means everything travelling the same gap is spread across parallel
	tracks instead of collapsing onto one line.
	*/
	const LANE_STEP = 1.1;
	const laneOf = new Map<string, number>();
	const channelGroups = new Map<string, string[]>();

	for (const edge of edgeList) {
		const id = `${edge.from}>${edge.to}`;
		const from = placements.get(edge.from);
		const to = placements.get(edge.to);
		if (!from || !to) continue;

		const key = `${from.y.toFixed(1)}>${to.y.toFixed(1)}`;
		channelGroups.set(key, [...(channelGroups.get(key) ?? []), id]);
	}

	for (const ids of channelGroups.values()) {
		// Centre the lane fan on the channel so a single edge stays dead
		// straight and a group spreads symmetrically either side of it.
		const sorted = [...ids].sort();
		const middle = (sorted.length - 1) / 2;
		sorted.forEach((id, index) => {
			laneOf.set(id, (index - middle) * LANE_STEP);
		});
	}

	const allBoxes = [...placements.values()].map((placement) => ({ id: placement.id, box: boxOf(placement) }));

	const xs = [...placements.values()].map((placement) => placement.x);
	const bounds = {
		minX: xs.length > 0 ? Math.min(...xs) : 0,
		maxX: xs.length > 0 ? Math.max(...xs) : 0,
	};

	for (const edge of edgeList) {
		const id = `${edge.from}>${edge.to}`;
		const from = placements.get(edge.from);
		const to = placements.get(edge.to);
		const side = sides.get(id);
		const fromPort = portsMap.get(`${id}:from`);
		const toPort = portsMap.get(`${id}:to`);
		if (!from || !to || !side || !fromPort || !toPort) continue;

		// An edge's own endpoints aren't obstacles — it has to touch them.
		const obstacles = allBoxes
			.filter((entry) => entry.id !== edge.from && entry.id !== edge.to)
			.map((entry) => entry.box);

		const points = routeEdge(
			fromPort,
			toPort,
			side.from,
			side.to,
			laneOf.get(id) ?? 0,
			obstacles,
			bounds,
		);

		edgesOut.push({
			id,
			from: edge.from,
			to: edge.to,
			d: pathOf(points),
			points,
			labelPoint: labelPointOf(points),
			stats: edge,
		});
	}

	/*
	extent is the drawn bounding box in layout units, padded for the outside
	detour tracks. The view maps this onto its viewBox, so the diagram scales
	to fit whatever size the topology turns out to be instead of being squeezed
	into a fixed 0–100 space.
	*/
	const ys = [...placements.values()].map((placement) => placement.y);
	const pad = HALF.w + CLEARANCE + 6;

	const extent =
		placements.size === 0
			? { x: 0, y: 0, w: 100, h: 100 }
			: {
					x: Math.min(...xs) - pad,
					y: Math.min(...ys) - HALF.h - 4,
					w: Math.max(...xs) - Math.min(...xs) + pad * 2,
					h: Math.max(...ys) - Math.min(...ys) + (HALF.h + 4) * 2,
				};

	return { placements, edges: edgesOut, ports, extent };
};

const pathsFrom = (selection: DiagnosticsSelection | null, edges: DiagEdge[]) => {
	if (selection === null) {
		return { upstream: new Set<string>(), downstream: new Set<string>() };
	}

	const outgoing = new Map<string, DiagEdge[]>();
	const incoming = new Map<string, DiagEdge[]>();

	for (const edge of edges) {
		outgoing.set(edge.from, [...(outgoing.get(edge.from) ?? []), edge]);
		incoming.set(edge.to, [...(incoming.get(edge.to) ?? []), edge]);
	}

	const walk = (start: string, direction: "up" | "down"): Set<string> => {
		const visited = new Set<string>([start]);
		const frontier = [start];

		while (frontier.length > 0) {
			const current = frontier.shift() as string;
			const candidates = direction === "up" ? incoming.get(current) : outgoing.get(current);

			for (const edge of candidates ?? []) {
				const next = direction === "up" ? edge.from : edge.to;
				if (visited.has(next)) continue;
				visited.add(next);
				frontier.push(next);
			}
		}

		visited.delete(start);
		return visited;
	};

	return { upstream: walk(selection.name, "up"), downstream: walk(selection.name, "down") };
};

type StageState = "live" | "stale" | "unseen";

const stageState = (stage: NodeStats | undefined, atNs: number): StageState => {
	if (stage === undefined) return "unseen";
	if (atNs - stage.lastAtNs <= TOPOLOGY_LIVE_WINDOW_NS) return "live";
	return "stale";
};

const STAGE_TONE: Record<StageState, { dot: string; borderColor: string; text: string }> = {
	live: { dot: "bg-(--up)", borderColor: "var(--up)", text: "text-(--up)" },
	stale: { dot: "bg-(--f4)", borderColor: "var(--f4)", text: "text-(--f4)" },
	unseen: { dot: "bg-(--line2)", borderColor: "var(--line)", text: "text-(--f4)" },
};

export type BacklogTone = "clear" | "building" | "backed-up";

/*
backlogTone reads current pressure against this stage's own session peak — a
ring's absolute capacity isn't known client-side, but "close to the worst
this stage has ever seen" is the same signal the old queue tanks' high-water
mark gave, and needs no configuration.
*/
export const backlogTone = (backlog: number, maxBacklog: number): BacklogTone => {
	if (backlog <= 0) return "clear";
	if (maxBacklog <= 0) return "building";

	const ratio = backlog / maxBacklog;

	if (ratio >= 0.7) return "backed-up";
	if (ratio >= 0.25) return "building";
	return "clear";
};

const BACKLOG_TONE_FILL: Record<BacklogTone, string> = {
	clear: "bg-(--up)",
	building: "bg-(--warn)",
	"backed-up": "bg-(--down)",
};

type Extent = { x: number; y: number; w: number; h: number };

// Layout units are abstract; HTML overlays position in percentages of the
// drawn extent, so both layers agree on where a point sits.
const toPercent = (point: Point, extent: Extent) => ({
	left: ((point.x - extent.x) / extent.w) * 100,
	top: ((point.y - extent.y) / extent.h) * 100,
});

const StageNode = ({
	placement,
	extent,
	stage,
	state,
	selected,
	dimmed,
	highlight,
	onSelect,
}: {
	placement: Placement;
	extent: Extent;
	stage: NodeStats | undefined;
	state: StageState;
	selected: boolean;
	dimmed: boolean;
	highlight: "up" | "down" | null;
	onSelect: (selection: DiagnosticsSelection) => void;
}) => {
	const tone = STAGE_TONE[state];
	const backlog = stage?.backlog ?? 0;
	const maxBacklog = stage?.maxBacklog ?? 0;
	const pressure = backlogTone(backlog, maxBacklog);
	const fillRatio = maxBacklog > 0 ? Math.min(1, backlog / maxBacklog) : 0;

	return (
		<button
			type="button"
			onClick={(event) => {
				event.stopPropagation();
				onSelect({ kind: "stage", name: placement.id });
			}}
			aria-label={`Inspect ${placement.label}`}
			className={cn(
				"diag-node absolute z-10 -translate-x-1/2 -translate-y-1/2 cursor-pointer overflow-hidden rounded-xs border bg-(--surface) px-2 py-1.5 text-left transition-all hover:bg-(--raised)",
				"flex flex-col justify-between",
				state === "live" && !dimmed && "diag-node-live",
				selected && "outline outline-(--acc) outline-offset-1 ring-1 ring-(--acc)/40",
				highlight === "up" && !selected && "outline outline-(--warn)/70 outline-offset-1",
				highlight === "down" && !selected && "outline outline-(--info)/70 outline-offset-1",
				dimmed && "opacity-20",
			)}
			style={{
				left: `${toPercent(placement, extent).left}%`,
				top: `${toPercent(placement, extent).top}%`,
				// A share of the extent, which the surface has already sized so
				// this lands at or above MIN_NODE_PX — the card scales with the
				// diagram without ever dropping below readable.
				width: `${((HALF.w * 2) / extent.w) * 100}%`,
				height: `${((HALF.h * 2) / extent.h) * 100}%`,
				borderColor: tone.borderColor,
			}}
		>
			{/* Ring backlog: how far this stage is behind its Workload's
			producer, relative to the worst it's seen this session. */}
			<span
				className={cn(
					"pointer-events-none absolute inset-x-0 bottom-0 block transition-all duration-300",
					BACKLOG_TONE_FILL[pressure],
				)}
				style={{ height: `${fillRatio * 100}%`, opacity: backlog > 0 ? 0.16 : 0 }}
			/>
			<div className="relative flex items-center gap-1">
				<span className={`size-1.5 shrink-0 rounded-full ${tone.dot}`} />
				<span
					className="truncate font-mono text-[11px] font-semibold uppercase tracking-wide text-(--f1)"
					title={placement.label}
				>
					{placement.label}
				</span>
			</div>
			{/* Columns are sized in fr rather than fixed ch so a long value
			borrows room from its neighbours instead of being clipped — the
			fixed 3ch/7ch/6ch grid truncated real readings like "peak 1.5K". */}
			<div className="relative grid grid-cols-[auto_1fr_auto] items-baseline gap-1.5 font-mono">
				<span className="text-[9px] uppercase text-(--f4)">rate</span>
				<span
					className={cn(
						"text-right text-[13px] font-bold tabular-nums text-(--acc)",
						stage === undefined && "text-(--f4)",
					)}
				>
					{stage === undefined ? "—" : formatRate(stage.avgGapNs)}
				</span>
				<span className={`text-right text-[9px] uppercase ${tone.text}`}>{state}</span>
			</div>
			<div className="relative grid grid-cols-[auto_1fr_auto] items-baseline gap-1.5 font-mono text-[9px] text-(--f3)">
				<span className="text-(--f4)">last</span>
				<span className="text-right tabular-nums">{formatNanos(stage?.lastGapNs)}</span>
				<span className="text-right tabular-nums">
					{stage !== undefined ? `${formatCount(stage.seqCount)} ops` : "unseen"}
				</span>
			</div>
			<div className="relative grid grid-cols-[auto_1fr_auto] items-baseline gap-1.5 font-mono text-[9px] text-(--f3)">
				<span className="text-(--f4)">bklg</span>
				<span
					className={cn(
						"text-right font-bold tabular-nums",
						pressure === "backed-up" && "text-(--down)",
						pressure === "building" && "text-(--warn)",
						pressure === "clear" && "text-(--f3)",
					)}
				>
					{formatCount(backlog)}
				</span>
				<span className="text-right tabular-nums">peak {formatCount(maxBacklog)}</span>
			</div>
		</button>
	);
};

const EdgePath = ({
	edge,
	flowing,
	dimmed,
	highlight,
	hovered,
	onHover,
}: {
	edge: DiagEdge;
	flowing: boolean;
	dimmed: boolean;
	highlight: "up" | "down" | null;
	hovered: boolean;
	onHover: (hovered: boolean) => void;
}) => {
	const health = edgeHealth(edge.stats.avgLatencyNs);
	const stroke =
		highlight === "up"
			? "var(--warn)"
			: highlight === "down"
				? "var(--info)"
				: hovered
					? "var(--acc)"
					: EDGE_HEALTH_STROKE[health];

	return (
		// biome-ignore lint/a11y/noStaticElementInteractions: Because I'm Batman.
		<g onMouseEnter={() => onHover(true)} onMouseLeave={() => onHover(false)} className="cursor-pointer">
			<title>{`${edge.from} → ${edge.to}`}</title>
			<path d={edge.d} fill="none" stroke="transparent" strokeWidth={6} vectorEffect="non-scaling-stroke" />
			<path
				d={edge.d}
				data-from={edge.from}
				data-to={edge.to}
				data-health={health}
				fill="none"
				stroke={stroke}
				strokeWidth={hovered ? 1.8 : 1.2}
				strokeOpacity={dimmed && !hovered ? 0.15 : flowing ? 0.4 : 0.6}
				vectorEffect="non-scaling-stroke"
				pathLength={100}
				strokeLinecap="round"
				strokeLinejoin="round"
				className="diag-edge transition-all"
			/>
			{/* On a live edge a bright dashed overlay rides the same path as the
			base stroke, so flow reads as motion along a still-visible wire
			rather than as the wire itself blinking in and out. */}
			{flowing && !dimmed && (
				<path
					d={edge.d}
					fill="none"
					stroke={stroke}
					strokeWidth={hovered ? 2.4 : 1.8}
					strokeOpacity={0.95}
					vectorEffect="non-scaling-stroke"
					strokeLinecap="round"
					strokeLinejoin="round"
					className="diag-flow"
				/>
			)}
		</g>
	);
};

const EdgeLatency = ({
	edge,
	extent,
	dimmed,
	hovered,
}: { edge: DiagEdge; extent: Extent; dimmed: boolean; hovered: boolean }) => {
	if (edge.stats.avgLatencyNs <= 0) return null;

	const at = toPercent(edge.labelPoint, extent);

	return (
		<div
			className={cn(
				"pointer-events-none absolute z-5 -translate-x-1/2 -translate-y-1/2 whitespace-nowrap rounded-sm border border-(--line) bg-(--bg)/95 px-1 py-px text-center font-mono text-[9px] tabular-nums text-(--f3) shadow-sm transition-all",
				dimmed && !hovered && "opacity-15",
				hovered && "border-(--acc) text-(--acc) opacity-100 z-20 scale-110",
			)}
			style={{ left: `${at.left}%`, top: `${at.top}%` }}
			title={`${edge.from} to ${edge.to} average hop latency`}
		>
			{formatNanos(edge.stats.avgLatencyNs)}
		</div>
	);
};

export type DiagnosticsGraphProps = {
	nodes: Map<string, NodeStats>;
	edges: Map<string, EdgeStats>;
	atNs: number;
	selection: DiagnosticsSelection | null;
	onSelect: (selection: DiagnosticsSelection | null) => void;
};

/*
DiagnosticsGraph renders the live pipeline topology as an auto-laid-out wiring
diagram, discovered entirely from Envelope.Boundaries stamps: nodes are the
distinct stage labels seen, edges are consecutive label pairs, and both
position and existence update as the process actually runs — nothing here is
declared ahead of time. Selecting a node highlights its upstream feeders
(amber) and downstream consumers (blue) while dimming the rest.
*/
export const DiagnosticsGraph = ({ nodes, edges, atNs, selection, onSelect }: DiagnosticsGraphProps) => {
	// nodes/edges are Maps mutated in place by topologyStore.ingest, so their
	// references never change — atNs (the freshest stamp timestamp seen) is
	// the actual "this data changed" signal the memo needs to depend on.
	// biome-ignore lint/correctness/useExhaustiveDependencies: nodes/edges are read for their mutated contents, not their (stable) reference — atNs is the real change signal.
	const graph = useMemo(() => buildDiagnosticsGraph(nodes, edges), [atNs]);
	const [hoveredEdgeId, setHoveredEdgeId] = useState<string | null>(null);

	const { upstream, downstream } = useMemo(() => pathsFrom(selection, graph.edges), [selection, graph.edges]);

	/*
	The diagram is drawn at whatever pixel size keeps a node card readable, and
	the container scrolls if that exceeds the viewport. Sizing the canvas to the
	viewport instead (the previous behaviour) shrank every card as stages were
	discovered, which is what made the metrics unreadable — and because the
	extent's aspect ratio tracks the topology's shape, a tall graph squashed
	cards in one axis only.
	*/
	const surface = {
		width: (graph.extent.w / (HALF.w * 2)) * MIN_NODE_PX.w,
		height: (graph.extent.h / (HALF.h * 2)) * MIN_NODE_PX.h,
	};

	if (nodes.size === 0) {
		return (
			<div className="flex h-full items-center justify-center font-mono text-[10px] uppercase tracking-widest text-(--f4)">
				Waiting for the pipeline to stamp its first boundary
			</div>
		);
	}

	return (
		// biome-ignore lint/a11y/noStaticElementInteractions: click-outside-to-deselect on the background; every real interaction (selecting a stage) has its own keyboard-accessible button.
		// biome-ignore lint/a11y/useKeyWithClickEvents: same as above — this is a convenience dismiss, not the primary interaction path.
		<div className="relative h-full w-full overflow-auto select-none" onClick={() => onSelect(null)}>
			<style>{`
				@keyframes diag-dash-flow {
					from { stroke-dashoffset: 18; }
					to { stroke-dashoffset: 0; }
				}
				@keyframes diag-node-live {
					0%, 100% { box-shadow: 0 0 0 0 var(--up); opacity: 1; }
					50% { box-shadow: 0 0 0 2px color-mix(in srgb, var(--up) 22%, transparent); opacity: 1; }
				}
				.diag-edge { stroke-linecap: round; }
				/* Dash pattern is in user units against pathLength-free geometry,
				   so it stays consistent whatever the route's length; the offset
				   runs source-to-target (positive to zero) to read as travel in
				   the direction of flow. */
				.diag-flow {
					stroke-dasharray: 3 15;
					animation: diag-dash-flow 1.1s linear infinite;
				}
				.diag-node-live::after {
					content: "";
					position: absolute;
					inset: -1px;
					border-radius: inherit;
					pointer-events: none;
					animation: diag-node-live 2s ease-in-out infinite;
				}
				@media (prefers-reduced-motion: reduce) {
					.diag-flow { animation: none; stroke-dasharray: none; stroke-opacity: 0.9; }
					.diag-node-live::after { animation: none; }
				}
			`}</style>
			{/* The sized surface: at least as large as readability demands, and
			at least as large as the viewport so a small topology still fills
			the panel rather than huddling in a corner. */}
			<div
				className="relative"
				style={{ width: `max(100%, ${Math.round(surface.width)}px)`, height: `max(100%, ${Math.round(surface.height)}px)` }}
			>
			{/* preserveAspectRatio="none" stretches the routes to match the HTML
			overlay, which is itself positioned in percentages of the same
			extent — both layers therefore land on identical geometry. */}
			<svg
				viewBox={`${graph.extent.x} ${graph.extent.y} ${graph.extent.w} ${graph.extent.h}`}
				preserveAspectRatio="none"
				className="absolute inset-0 h-full w-full"
				aria-hidden="true"
			>
				<g className="diag-edges">
					{graph.edges.map((edge) => {
						const flowing = atNs - edge.stats.lastAtNs <= TOPOLOGY_LIVE_WINDOW_NS;
						const highlightUp = selection !== null && (upstream.has(edge.from) || upstream.has(edge.to));
						const highlightDown = selection !== null && (downstream.has(edge.from) || downstream.has(edge.to));
						const highlight = highlightUp ? ("up" as const) : highlightDown ? ("down" as const) : null;
						const dimmed = selection !== null && highlight === null;
						const hovered = hoveredEdgeId === edge.id;

						return (
							<EdgePath
								key={edge.id}
								edge={edge}
								flowing={flowing}
								dimmed={dimmed}
								highlight={highlight}
								hovered={hovered}
								onHover={(isHovered) => setHoveredEdgeId(isHovered ? edge.id : null)}
							/>
						);
					})}
				</g>
			</svg>

			{graph.edges.map((edge) => {
				const connected =
					selection?.name === edge.from ||
					selection?.name === edge.to ||
					upstream.has(edge.from) ||
					upstream.has(edge.to) ||
					downstream.has(edge.from) ||
					downstream.has(edge.to);
				const hovered = hoveredEdgeId === edge.id;

				return (
					<EdgeLatency
						key={`latency:${edge.id}`}
						edge={edge}
						extent={graph.extent}
						dimmed={selection !== null && !connected}
						hovered={hovered}
					/>
				);
			})}

			{Array.from(graph.placements.values()).map((placement) => {
				const selectedHere = selection?.name === placement.id;
				const isUp = upstream.has(placement.id);
				const isDown = downstream.has(placement.id);
				const highlight = selectedHere ? null : isUp ? ("up" as const) : isDown ? ("down" as const) : null;
				const dimmed = selection !== null && !selectedHere && highlight === null;
				const stage = nodes.get(placement.id);

				return (
					<StageNode
						key={placement.id}
						placement={placement}
						extent={graph.extent}
						stage={stage}
						state={stageState(stage, atNs)}
						selected={selectedHere}
						dimmed={dimmed}
						highlight={highlight}
						onSelect={onSelect}
					/>
				);
			})}
			</div>
		</div>
	);
};
