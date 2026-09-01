import { useEffect, useMemo, useRef, useState } from "react";
import type { EdgeStats, NodeStats } from "#/collections/topology";
import { TOPOLOGY_LIVE_WINDOW_NS } from "#/collections/topology";
import { cn } from "@/lib/utils";

/*
DiagnosticsSelection identifies what the wiring graph currently has selected:
one stage node, or one whole ring. The detail rail keys off the same value.
*/
export type DiagnosticsSelection =
	| { kind: "stage"; name: string }
	| { kind: "group"; name: string };

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
export const HALF = { w: 9.4, h: 5.2 };
const COL_PITCH = 21;
const ROW_PITCH = 14;

/*
EXTENT_PAD is the margin the drawn extent leaves around the packed content,
for the outside tracks a detouring edge uses. It is small because those tracks
are not far out: a detour clears the outermost card by roughly HALF plus the
clearance, and a ring's own box already extends further than that past the
cards inside it. The packing search needs the same number to predict the card
size a candidate would actually render at.
*/
const EXTENT_PAD = { x: 5, y: 4 };

/*
MIN_NODE_PX is the floor below which a card stops being a card at all — the
point where even its name no longer fits. The surface only scrolls if the
topology cannot reach even this, which the clustered layout makes rare: the
packing is chosen to fit this panel, so it normally fits outright.

Between that floor and a comfortable card the view drops metric rows rather
than shrinking type (see NODE_DENSITY), because a smaller reading of every
number is worse than fewer numbers each still legible.
*/
const MIN_NODE_PX = { w: 64, h: 26 };

/*
NODE_DENSITY picks how much of a stage's card survives at the height the
diagram actually ended up with. Each tier keeps the readings in order of what
a glance needs first: which stage it is, how fast it is running, how recently,
and only then how far behind its ring it has fallen.
*/
type NodeDensity = "full" | "compact" | "minimal" | "name";

/*
NODE_TYPE is each tier's own type scale and padding. A tier is chosen by the
height it needs, so these two have to be read together: the thresholds below
are the sum of the rows a tier draws at the sizes set here, plus its padding
and a little slack. Sizing them apart is how a tier ends up chosen for a card
it cannot fit in, which clips the reading it was chosen to show.
*/
const NODE_TYPE: Record<
	NodeDensity,
	{ pad: string; title: string; rate: string; meta: string; gap: string }
> = {
	full: {
		pad: "px-2 py-1.5",
		title: "text-[11px]",
		rate: "text-[12px]",
		meta: "text-[9px]",
		gap: "gap-y-0.5",
	},
	compact: {
		pad: "px-2 py-1",
		title: "text-[10px]",
		rate: "text-[11px]",
		meta: "text-[9px]",
		gap: "gap-y-0",
	},
	minimal: {
		pad: "px-1.5 py-1",
		title: "text-[10px]",
		rate: "text-[11px]",
		meta: "text-[8px]",
		gap: "gap-y-0",
	},
	name: {
		pad: "px-1.5 py-0.5",
		title: "text-[10px]",
		rate: "text-[10px]",
		meta: "text-[8px]",
		gap: "gap-y-0",
	},
};

/*
densityFor picks the richest tier that actually fits the card the layout ended
up with. Each threshold is what that tier measures at in NODE_TYPE — its rows
at 1.15 leading plus its own padding and row gaps — with a few pixels of slack
so a reading never sits hard against the border. Guessing the thresholds
independently of the type scale is how a tier gets chosen for a card it cannot
fit in, and the reading it was chosen to show is the one that gets clipped.
*/
const densityFor = (heightPx: number): NodeDensity => {
	// full: 4 rows + 12px padding + 6px of row gaps ≈ 66px
	if (heightPx >= 68) return "full";
	// compact: 3 rows + 8px padding ≈ 44px
	if (heightPx >= 46) return "compact";
	// minimal: 2 rows + 8px padding ≈ 33px
	if (heightPx >= 35) return "minimal";
	return "name";
};

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
	// group is the runtime ring this stage belongs to, carried through so the
	// view can label a card by its short name within its own cluster instead
	// of repeating the ring's name on every box inside it.
	group: string;
	x: number;
	y: number;
};

/*
GroupBox is one ring drawn as an enclosure around its own stages, in the same
layout units as the placements. headerH is the strip at the top reserved for
the ring's name, so the box's title never lands on the first row of cards.
*/
export type GroupBox = {
	id: string;
	label: string;
	x: number;
	y: number;
	w: number;
	h: number;
	headerH: number;
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
A Cluster is one runtime ring drawn as a unit: its member stages laid out in
rows, and the box those rows sit in. `rows` is the ring's own handler-group
order when the ring reported one (see runtime.Composed), which is the only
way to know that several stages run side by side behind a single barrier —
a trace of stamps cannot say so, because concurrent siblings stamp in
whatever order their goroutines finish.
*/
type Cluster = {
	id: string;
	label: string;
	rows: string[][];
	// Cell extents. A cell is one COL_PITCH x ROW_PITCH grid square, sized
	// so a node box plus its edge channel fits inside one.
	w: number;
	h: number;
	x: number;
	y: number;
};

/*
GROUP_WRAPS are the widths a concurrent stage may be folded to inside its own
cluster. Siblings behind one barrier have no order between them, so wrapping
them costs no meaning — a six-wide fan-out folded to two rows of three still
says exactly what it said. Which width to use is not fixed here: it trades
diagram width against diagram height, and only the packing search knows which
of the two the panel is short of.
*/
const GROUP_WRAPS = [2, 3, 4, 5, 6, 8];
const GROUP_PAD = 0.2;
const GROUP_HEADER = 0.2;
const GROUP_GAP = 0.25;

/*
IDEAL_NODE_PX is the card the layout aims for: wide enough for a stage name
and a rate side by side, tall enough for the metric rows under them. The
packing search below maximises how close the smaller of the two dimensions
gets to this, which is what balances a diagram that is too wide against one
that is too tall — neither is any use if the cards end up illegible.
*/
const IDEAL_NODE_PX = { w: 150, h: 60 };

/*
concurrentSiblings reports whether two stages run side by side in the same
handler group. Their boundary stamps land in goroutine-completion order, so
the topology store sees an "edge" between them in whichever direction won the
race that envelope — usually both directions, over time. Those are not hops
and drawing them is what turned the real pipeline into a hairball; they are
dropped from the layout and from the wiring alike.
*/
export const concurrentSiblings = (
	a: NodeStats | undefined,
	b: NodeStats | undefined,
): boolean =>
	a !== undefined &&
	b !== undefined &&
	a.group !== "" &&
	a.group === b.group &&
	a.stage === b.stage;

/* isHop keeps the edges that describe real flow between distinct stages. */
export const isHop = (
	nodes: Map<string, NodeStats>,
	edge: EdgeStats,
): boolean =>
	edge.from !== edge.to &&
	!concurrentSiblings(nodes.get(edge.from), nodes.get(edge.to));

/*
longestPathRanks is the fallback row assignment for stages whose ring never
reported a handler group — a node's rank is the longest path reaching it from
a source, the classic Sugiyama layering. Grouped stages don't need it: their
ring already knows which barrier each one sits behind.
*/
const longestPathRanks = (
	ids: string[],
	incoming: Map<string, string[]>,
): Map<string, number> => {
	const rank = new Map<string, number>();
	const within = new Set(ids);
	const visiting = new Set<string>();

	const resolve = (id: string): number => {
		const cached = rank.get(id);
		if (cached !== undefined) return cached;
		// A cycle should not exist in a pipeline, but a stray self-referential
		// hop must not spin forever — the node caught mid-cycle just keeps
		// whatever rank it was first reached at.
		if (visiting.has(id)) return 0;

		visiting.add(id);
		const parents = (incoming.get(id) ?? []).filter((parent) =>
			within.has(parent),
		);
		const resolved =
			parents.length === 0
				? 0
				: Math.max(...parents.map((parent) => resolve(parent) + 1));
		visiting.delete(id);
		rank.set(id, resolved);

		return resolved;
	};

	for (const id of ids) resolve(id);

	return rank;
};

/*
clustersOf partitions the stages into one Cluster per ring and lays each one
out internally: one row per handler group, in barrier order, wrapping a wide
concurrent stage rather than letting it set the width of the whole diagram.
Stages from rings that reported no group fall into a single unnamed cluster
laid out by longest path, so a topology carrying no composition information at
all still renders exactly as it always did.
*/
const clustersOf = (
	nodeIds: string[],
	nodes: Map<string, NodeStats>,
	incoming: Map<string, string[]>,
	wrap: number,
): Cluster[] => {
	const byGroup = new Map<string, string[]>();

	for (const id of nodeIds) {
		const group = nodes.get(id)?.group ?? "";
		byGroup.set(group, [...(byGroup.get(group) ?? []), id]);
	}

	const clusters: Cluster[] = [];

	for (const [group, ids] of byGroup) {
		const ranks =
			group === ""
				? longestPathRanks(ids, incoming)
				: new Map(ids.map((id) => [id, nodes.get(id)?.stage ?? 0]));

		const byRank = new Map<number, string[]>();

		for (const id of ids) {
			const rank = ranks.get(id) ?? 0;
			byRank.set(rank, [...(byRank.get(rank) ?? []), id]);
		}

		const rows: string[][] = [];

		for (const rank of [...byRank.keys()].sort((a, b) => a - b)) {
			const members = [...(byRank.get(rank) ?? [])].sort();

			// A stage wider than the wrap becomes several rows of siblings.
			// They are unordered with respect to each other, so this splits a
			// set, it does not impose a sequence.
			for (let start = 0; start < members.length; start += wrap) {
				rows.push(members.slice(start, start + wrap));
			}
		}

		const content = Math.max(1, ...rows.map((row) => row.length));

		clusters.push({
			id: group,
			label: group,
			rows,
			w: content + GROUP_PAD * 2,
			h: rows.length + GROUP_PAD * 2 + GROUP_HEADER,
			x: 0,
			y: 0,
		});
	}

	return clusters;
};

/*
clusterLayers ranks whole rings against each other the same way stages are
ranked within one: a ring's layer is the longest path of cross-ring hops
reaching it. Rings nothing feeds — the ingress rings — therefore land in
layer 0, which is what puts ingress at the top of the diagram without anyone
naming which rings those are.
*/
const clusterLayers = (
	clusters: Cluster[],
	nodes: Map<string, NodeStats>,
	edges: EdgeStats[],
): Map<string, number> => {
	const groupOf = (id: string) => nodes.get(id)?.group ?? "";
	const parents = new Map<string, string[]>();

	for (const edge of edges) {
		const from = groupOf(edge.from);
		const to = groupOf(edge.to);
		if (from === to) continue;
		parents.set(to, [...(parents.get(to) ?? []), from]);
	}

	return longestPathRanks(
		clusters.map((cluster) => cluster.id),
		parents,
	);
};

/*
packLayers places the clusters of each layer left to right, wrapping onto a
further row once a layer would exceed `width` cells, and stacking the layers
downward. Returning the packed height lets the caller try several widths and
keep the one whose shape matches the panel — the diagram has to fit without
scrolling, and only the caller knows what it has to fit into.
*/
const packLayers = (layers: Cluster[][], width: number): number => {
	let cursor = 0;

	for (const layer of layers) {
		let row: Cluster[] = [];
		let rowWidth = 0;

		const flush = () => {
			if (row.length === 0) return;

			const rowHeight = Math.max(...row.map((cluster) => cluster.h));
			// Centre the row so a layer with one wide ring and a layer with
			// three narrow ones still read as the same column of flow.
			let x = (width - rowWidth) / 2;

			for (const cluster of row) {
				cluster.x = x;
				cluster.y = cursor;
				x += cluster.w + GROUP_GAP;
			}

			cursor += rowHeight + GROUP_GAP;
			row = [];
			rowWidth = 0;
		};

		for (const cluster of layer) {
			const added =
				rowWidth === 0 ? cluster.w : rowWidth + GROUP_GAP + cluster.w;

			if (row.length > 0 && added > width) flush();

			rowWidth =
				row.length === 0 ? cluster.w : rowWidth + GROUP_GAP + cluster.w;
			row.push(cluster);
		}

		flush();
	}

	return Math.max(cursor - GROUP_GAP, 1);
};

/*
autoLayout positions every stage from the composition the rings report and the
hops envelopes actually took — no coordinate is hand-authored and no label is
special-cased. Rings become clusters, cluster layers stack top to bottom in
flow order, and `panel` (the space in pixels the diagram has to fill) chooses
how wide to pack them.
*/
const autoLayout = (
	nodeIds: string[],
	nodes: Map<string, NodeStats>,
	edges: EdgeStats[],
	panel: { w: number; h: number },
): { placements: Map<string, Placement>; groups: GroupBox[] } => {
	const outgoing = new Map<string, string[]>();
	const incoming = new Map<string, string[]>();

	for (const edge of edges) {
		outgoing.set(edge.from, [...(outgoing.get(edge.from) ?? []), edge.to]);
		incoming.set(edge.to, [...(incoming.get(edge.to) ?? []), edge.from]);
	}

	const layerOf = clusterLayers(
		clustersOf(nodeIds, nodes, incoming, GROUP_WRAPS[0]),
		nodes,
		edges,
	);

	// Widest ring first within a layer: a big cluster placed after several
	// small ones is what forces an early wrap and leaves a ragged row.
	const layersOf = (clusters: Cluster[]): Cluster[][] => {
		const indices = [
			...new Set(clusters.map((cluster) => layerOf.get(cluster.id) ?? 0)),
		].sort((a, b) => a - b);

		return indices.map((layer) =>
			clusters
				.filter((cluster) => (layerOf.get(cluster.id) ?? 0) === layer)
				.sort((a, b) => b.w - a.w || a.id.localeCompare(b.id)),
		);
	};

	/*
	Search every (wrap, width) packing for the one that leaves the biggest
	legible card. Scoring on the resulting card size rather than on the
	diagram's outline is the point: a packing can match the panel's shape
	exactly and still be mostly empty space, and it is the card — not the
	outline — the reader has to be able to read. Taking the smaller of the two
	dimensions against the ideal card balances a packing that is too wide
	against one that is too tall, since the surface stretches each axis
	independently.

	Fixing both instead (the previous layout spread every layer across a
	single row at a fixed fan-out width) is what made the real topology
	several panels tall, so the surface had to scroll and the ingress stages
	were never on screen with the strategy stages they feed.
	*/
	let chosen: {
		clusters: Cluster[];
		layers: Cluster[][];
		width: number;
	} | null = null;
	let best = Number.NEGATIVE_INFINITY;

	for (const wrap of GROUP_WRAPS) {
		const clusters = clustersOf(nodeIds, nodes, incoming, wrap);
		const layers = layersOf(clusters);

		const widest = Math.max(1, ...clusters.map((cluster) => cluster.w));
		const total = clusters.reduce(
			(sum, cluster) => sum + cluster.w + GROUP_GAP,
			0,
		);

		for (let candidate = widest; candidate <= total; candidate += 0.5) {
			const packed = packLayers(layers, candidate);
			// The drawn extent adds the same padding buildDiagnosticsGraph
			// does, so the card scored here is the card that actually renders.
			const cardW =
				(HALF.w * 2 * panel.w) / (candidate * COL_PITCH + EXTENT_PAD.x * 2);
			const cardH =
				(HALF.h * 2 * panel.h) / (packed * ROW_PITCH + EXTENT_PAD.y * 2);
			const score = Math.min(cardW / IDEAL_NODE_PX.w, cardH / IDEAL_NODE_PX.h);

			if (score > best) {
				best = score;
				chosen = { clusters, layers, width: candidate };
			}
		}
	}

	// GROUP_WRAPS is never empty, so the search always settles on something;
	// the fallback only satisfies the type.
	const { clusters, layers, width } = chosen ?? {
		clusters: clustersOf(nodeIds, nodes, incoming, GROUP_WRAPS[0]),
		layers: layersOf(clustersOf(nodeIds, nodes, incoming, GROUP_WRAPS[0])),
		width: 1,
	};

	packLayers(layers, width);

	const placements = new Map<string, Placement>();

	for (const cluster of clusters) {
		const content = cluster.w - GROUP_PAD * 2;
		const originX = cluster.x + GROUP_PAD;
		const originY = cluster.y + GROUP_PAD + GROUP_HEADER;

		cluster.rows.forEach((row, rowIndex) => {
			const indent = (content - row.length) / 2;

			row.forEach((id, index) => {
				placements.set(id, {
					id,
					label: id,
					group: cluster.id,
					x: (originX + indent + index + 0.5) * COL_PITCH,
					y: (originY + rowIndex + 0.5) * ROW_PITCH,
				});
			});
		});
	}

	/*
	Order siblings within each row by where their feeders sit. Rows come from
	the rings themselves so the vertical order is already correct; this only
	decides left-to-right within a row, which is exactly the freedom a set of
	concurrent siblings leaves open — and using it is what stops their inbound
	wires crossing each other on the way in.
	*/
	for (let pass = 0; pass < 4; pass++) {
		for (const cluster of clusters) {
			const content = cluster.w - GROUP_PAD * 2;
			const originX = cluster.x + GROUP_PAD;

			cluster.rows.forEach((row, rowIndex) => {
				if (row.length < 2) return;

				const barycenter = new Map<string, number>();

				row.forEach((id, index) => {
					const neighbours = [
						...(incoming.get(id) ?? []),
						...(outgoing.get(id) ?? []),
					]
						.map((other) => placements.get(other))
						.filter((other): other is Placement => other !== undefined);

					// No neighbour placed yet means no opinion — keep the
					// current index rather than collapsing to the left edge.
					barycenter.set(
						id,
						neighbours.length === 0
							? (originX + index + 0.5) * COL_PITCH
							: neighbours.reduce((sum, other) => sum + other.x, 0) /
									neighbours.length,
					);
				});

				const ordered = [...row].sort(
					(a, b) =>
						(barycenter.get(a) ?? 0) - (barycenter.get(b) ?? 0) ||
						a.localeCompare(b),
				);

				cluster.rows[rowIndex] = ordered;

				const indent = (content - ordered.length) / 2;

				ordered.forEach((id, index) => {
					const placement = placements.get(id);
					if (placement === undefined) return;
					placement.x = (originX + indent + index + 0.5) * COL_PITCH;
				});
			});
		}
	}

	const groups: GroupBox[] = clusters
		// The unnamed cluster holds stages whose ring reported nothing; it is
		// a bucket, not a ring, so it gets no box of its own to sit in.
		.filter((cluster) => cluster.id !== "")
		.map((cluster) => ({
			id: cluster.id,
			label: cluster.label,
			x: cluster.x * COL_PITCH,
			y: cluster.y * ROW_PITCH,
			w: cluster.w * COL_PITCH,
			h: cluster.h * ROW_PITCH,
			headerH: (GROUP_PAD + GROUP_HEADER) * ROW_PITCH,
		}));

	return { placements, groups };
};

const sidesFor = (
	from: Placement,
	to: Placement,
): { from: NodeSide; to: NodeSide } => {
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

	if (side === "top")
		return { x: placement.x + offset, y: placement.y - HALF.h };
	if (side === "bottom")
		return { x: placement.x + offset, y: placement.y + HALF.h };
	if (side === "left")
		return { x: placement.x - HALF.w, y: placement.y + offset };
	return { x: placement.x + HALF.w, y: placement.y + offset };
};

/*
STUB is how far a route runs straight out of a port before it may turn. It has
to stay inside the channel between two rows: a stub longer than half that
channel puts the first horizontal run inside the next row's clearance, and
every route that tries to escape sideways then reads as cutting through the
stage below its source.
*/
const STUB = 1.6;

const stubPoint = (point: Point, side: NodeSide, depth = STUB): Point => {
	if (side === "top") return { x: point.x, y: point.y - depth };
	if (side === "bottom") return { x: point.x, y: point.y + depth };
	if (side === "left") return { x: point.x - depth, y: point.y };
	return { x: point.x + depth, y: point.y };
};

/*
STUB_STEP is how much further out each successive port on one side turns. Every
wire attached to the same side used to turn at the same depth, so the runs they
made along that side were collinear — a dozen hops converging on one stage
approached it as a single thick line with a dozen wires hidden inside it.
Staggering the turn gives each its own track for as far as the channel allows.
*/
const STUB_STEP = 1.1;

/*
stubRoomOf measures how far a route may run straight out of one side of a node
before it reaches whatever sits beyond it. Ports stagger their turn within that
room, so how much staggering is available is a property of the actual layout —
a stage with a ring header above it has room for a real fan, one packed tight
under its neighbour has none, and neither case needs a constant to say so.
*/
const stubRoomOf = (
	placement: Placement,
	side: NodeSide,
	obstacles: { id: string; box: Box }[],
): number => {
	const vertical = side === "top" || side === "bottom";
	const sign = side === "top" || side === "left" ? -1 : 1;
	const reach = vertical ? HALF.h : HALF.w;

	let room = Number.POSITIVE_INFINITY;

	for (const entry of obstacles) {
		if (entry.id === placement.id) continue;

		// Only what lies directly beyond this side can limit the stub: an
		// obstacle off to one side is something the route turns to avoid, not
		// something it runs into on the way out.
		const across = vertical
			? Math.abs(entry.box.x - placement.x) < entry.box.w + HALF.w
			: Math.abs(entry.box.y - placement.y) < entry.box.h + HALF.h;

		if (!across) continue;

		const gap = vertical
			? sign * (entry.box.y - placement.y) - entry.box.h - reach
			: sign * (entry.box.x - placement.x) - entry.box.w - reach;

		if (gap >= 0) room = Math.min(room, gap);
	}

	return Number.isFinite(room) ? room : STUB + STUB_STEP * 4;
};

// Routes are drawn as hard orthogonal polylines — square corners read as
// circuit wiring, which is what this diagram is.
const pathOf = (points: Point[]): string =>
	points
		.map(
			(point, index) =>
				`${index === 0 ? "M" : "L"} ${point.x.toFixed(3)} ${point.y.toFixed(3)}`,
		)
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
const CLEARANCE = 0.7;

type Box = { x: number; y: number; w: number; h: number };

/*
A Corridor is one vertical lane a route may travel down: `x` its centre line
and `half` how far either side of that a wire may still be offset without
touching the cards on either flank.
*/
type Corridor = { x: number; half: number };

/*
CORRIDOR_CANDIDATES caps how many gaps a single hop tries. The list is sorted
by how close the gap is to the hop itself, so the first few are the ones worth
considering; scoring every gap in a large diagram would cost far more than it
could ever buy.
*/
const CORRIDOR_CANDIDATES = 4;

/*
CRAMPED_ROOM is the depth below which a side's stub fan can no longer separate
the wires attached to it — roughly the channel between two adjacent rows once
each one's clearance is taken out. Beyond it there is enough room to fan, and
a long run alongside that side stays legible.
*/
const CRAMPED_ROOM = 4;

/*
corridorsOf finds the vertical lanes between card columns — every gap wide
enough to take a wire clear of the cards on both sides, plus the diagram's own
two margins. Without them a hop that has to cross several rows has exactly one
escape, around the outside of everything, and enough such hops lay full-width
horizontal runs straight across the diagram.
*/
const corridorsOf = (placements: Map<string, Placement>): Corridor[] => {
	const columns = [
		...new Set([...placements.values()].map((placement) => placement.x)),
	].sort((a, b) => a - b);

	const reach = HALF.w + CLEARANCE;

	if (columns.length === 0) return [{ x: 0, half: 0 }];

	const corridors: Corridor[] = [
		{ x: columns[0] - reach - 2, half: 1 },
		{ x: columns[columns.length - 1] + reach + 2, half: 1 },
	];

	for (let i = 1; i < columns.length; i++) {
		const gap = columns[i] - columns[i - 1] - reach * 2;

		// A gap narrower than a wire's own clearance is not a corridor; two
		// columns that tight are effectively one solid wall.
		if (gap <= 0.5) continue;

		corridors.push({ x: (columns[i] + columns[i - 1]) / 2, half: gap / 2 });
	}

	return corridors;
};

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
	// How far each end runs straight out before it may turn. Staggered per
	// port so wires converging on one side of a stage do not all turn on the
	// same line.
	stubs: { from: number; to: number },
	// The vertical lanes a route may drop through, nearest-first candidates
	// being tried before the ones out at the diagram's margins.
	corridors: Corridor[],
	obstacles: Box[],
	// Ring enclosures a route should stay out of. Cutting through one is far
	// less wrong than cutting through a card — the box is empty space — but a
	// wire that crosses a ring it has no business in reads as if it belonged
	// to that ring, so it is avoided whenever an alternative exists.
	enclosures: Box[],
	// How much room each end has beyond its own edge. A long horizontal run
	// has to happen somewhere; this is what decides which end it happens at.
	rooms: { from: number; to: number },
): Point[] => {
	const start = stubPoint(fromPort, fromSide, stubs.from);
	const end = stubPoint(toPort, toSide, stubs.to);

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

		/*
		Drop through a corridor: out of the source, down a gap between card
		columns, and into the target. The corridors nearest the hop are tried
		first, and the diagram's own margins are simply the outermost two — so
		a hop that has to clear several rows takes the nearest gap rather than
		travelling around the entire diagram, which is what laid full-width
		horizontal runs across the middle of the graph.
		*/
		const middle = (start.x + end.x) / 2;
		const nearest = [...corridors]
			.sort((a, b) => Math.abs(a.x - middle) - Math.abs(b.x - middle))
			.slice(0, CORRIDOR_CANDIDATES);

		for (const corridor of nearest) {
			// The lane spreads parallel hops across the corridor's own width;
			// a narrow gap between two cards in one row simply takes them all
			// down its centre line.
			const offset = Math.max(-corridor.half, Math.min(corridor.half, lane));
			const track = corridor.x + offset;

			candidates.push([
				fromPort,
				start,
				{ x: track, y: start.y },
				{ x: track, y: end.y },
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

		// Route above or below the row rather than straight along it — again
		// with the lane widening the detour rather than narrowing it.
		for (const side of [-1, 1]) {
			const over = start.y + side * (HALF.h + CLEARANCE + 2 + Math.abs(lane));
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

	/*
	crampedTravel is how far a route runs horizontally in a channel with no
	room to separate it from its neighbours. Two hops that both make a long
	run through the same tight gap between rows end up drawn a pixel apart,
	which reads as one wire — whereas the same travel done where a fan has
	room stays legibly separate. The long leg of a route has to happen at one
	end or the other, so this makes that a scored choice rather than a
	coincidence of which candidate happened to be shorter.
	*/
	const crampedTravel = (points: Point[]) => {
		let cramped = 0;

		for (let i = 1; i < points.length; i++) {
			if (Math.abs(points[i].y - points[i - 1].y) > 0.001) continue;

			const run = Math.abs(points[i].x - points[i - 1].x);
			const nearFrom = Math.abs(points[i].y - start.y) < 0.001;
			const nearTo = Math.abs(points[i].y - end.y) < 0.001;

			if (nearFrom && rooms.from < CRAMPED_ROOM) cramped += run;
			else if (nearTo && rooms.to < CRAMPED_ROOM) cramped += run;
		}

		return cramped;
	};

	const lengthOf = (points: Point[]) => {
		let total = 0;
		for (let i = 1; i < points.length; i++) {
			total +=
				Math.abs(points[i].x - points[i - 1].x) +
				Math.abs(points[i].y - points[i - 1].y);
		}
		return total;
	};

	let best = candidates[0];
	let bestScore = Number.POSITIVE_INFINITY;

	for (const candidate of candidates) {
		// Hits dominate: a route that misses every node beats any shorter
		// route that cuts through one.
		const score =
			routeHitCount(candidate, obstacles) * 1000 +
			routeHitCount(candidate, enclosures) * 120 +
			candidate.length * 4 +
			lengthOf(candidate) * 0.05 +
			crampedTravel(candidate) * 0.6;

		if (score < bestScore) {
			bestScore = score;
			best = candidate;
		}
	}

	return best;
};

/*
DEFAULT_PANEL is the space assumed before the real one has measured itself —
roughly a wide dashboard pane, so the first paint is already close to the
final packing and the diagram does not visibly reflow once the true size
arrives.
*/
export const DEFAULT_PANEL = { w: 1280, h: 820 };

export const buildDiagnosticsGraph = (
	nodes: Map<string, NodeStats>,
	edges: Map<string, EdgeStats>,
	panel: { w: number; h: number } = DEFAULT_PANEL,
) => {
	const nodeIds = Array.from(nodes.keys());
	// Sibling stamps from one handler group are dropped before anything else
	// sees them: they are the same barrier reporting itself in race order, so
	// keeping them would both tangle the wiring and push concurrent stages
	// onto separate layers as though they fed each other.
	const edgeList = Array.from(edges.values()).filter((edge) =>
		isHop(nodes, edge),
	);
	const { placements, groups } = autoLayout(nodeIds, nodes, edgeList, panel);

	const attachments = new Map<
		string,
		{ edge: EdgeStats; end: "from" | "to"; opposite: number }[]
	>();
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
			{
				edge,
				end: "from",
				opposite:
					edgeSides.from === "top" || edgeSides.from === "bottom" ? to.x : to.y,
			},
		]);
		attachments.set(toKey, [
			...(attachments.get(toKey) ?? []),
			{
				edge,
				end: "to",
				opposite:
					edgeSides.to === "top" || edgeSides.to === "bottom" ? from.x : from.y,
			},
		]);
	}

	const portsMap = new Map<string, Point>();
	const stubsMap = new Map<string, number>();
	const roomsMap = new Map<string, number>();
	const ports: DiagPort[] = [];

	// Built here rather than at routing time because the stub fan needs to
	// know what sits beyond each side before any route is chosen.
	const boxes = [...placements.values()].map((placement) => ({
		id: placement.id,
		box: boxOf(placement),
	}));

	/*
	Where each wire attaches along a node's edge is decided per node — that is
	the node's own edge to share out. How deep each one runs before it turns is
	decided per row, below, because a depth is only distinguishable from the
	depths its neighbours chose.
	*/
	type Turn = { key: string; placement: Placement; side: NodeSide; at: number };
	const turns: Turn[] = [];

	for (const group of attachments.values()) {
		group.sort(
			(a, b) =>
				a.opposite - b.opposite ||
				`${a.edge.from}>${a.edge.to}`.localeCompare(
					`${b.edge.from}>${b.edge.to}`,
				),
		);

		group.forEach((attachment, index) => {
			const id = `${attachment.edge.from}>${attachment.edge.to}`;
			const side = sides.get(id);
			if (!side) return;

			const placement = placements.get(
				attachment.end === "from" ? attachment.edge.from : attachment.edge.to,
			);
			if (!placement) return;

			const nodeSide = attachment.end === "from" ? side.from : side.to;
			const point = portPoint(placement, nodeSide, index, group.length);
			const key = `${id}:${attachment.end}`;

			portsMap.set(key, point);
			turns.push({ key, placement, side: nodeSide, at: point.x + point.y });
			ports.push({
				id: key,
				edgeId: id,
				nodeId: placement.id,
				kind: attachment.end === "from" ? "out" : "in",
				point,
				side: nodeSide,
				latencyNs: attachment.edge.avgLatencyNs,
			});
		});
	}

	/*
	Fan the turns out per row and side rather than per node. Per node, a stage
	whose only hop leaves on its own always turned at exactly STUB — so every
	such hop along a row ran at one identical depth, and the moment two of them
	overlapped in x their tracks merged into a single line carrying both.
	Sharing the fan across the row gives each its own depth.

	How much depth there is to share comes from the layout itself: a row with
	the space of a ring header beyond it fans wide, one packed under its
	neighbour barely fans at all, and neither case needs a constant to say so.
	*/
	const fans = new Map<string, Turn[]>();

	for (const turn of turns) {
		const vertical = turn.side === "top" || turn.side === "bottom";
		// Everything on one side of one row shares a fan: the row is what the
		// depths have to be distinguishable across.
		const line = vertical ? turn.placement.y : turn.placement.x;
		const key = `${turn.side}:${line.toFixed(2)}`;
		fans.set(key, [...(fans.get(key) ?? []), turn]);
	}

	for (const fan of fans.values()) {
		fan.sort((a, b) => a.at - b.at || a.key.localeCompare(b.key));

		// The tightest member bounds the whole fan, so it stays even and no
		// wire is pushed past what its own node has room for.
		const room = Math.min(
			...fan.map((turn) => stubRoomOf(turn.placement, turn.side, boxes)),
		);

		// The fan starts at STUB and grows outward, so what is left for it is
		// the room beyond that first turn — not the whole channel, which would
		// push the outermost wires straight into whatever bounds the side.
		const span = Math.max(
			0,
			Math.min(room - 0.6 - STUB, STUB_STEP * (fan.length - 1)),
		);
		const step = fan.length > 1 ? span / (fan.length - 1) : 0;

		fan.forEach((turn, index) => {
			stubsMap.set(turn.key, STUB + index * step);
			roomsMap.set(turn.key, room);
		});
	}

	const edgesOut: DiagEdge[] = [];

	/*
	Edges crossing the same channel get distinct lanes. Grouping by the pair of
	layers an edge spans (rounded, since a layer's y is shared by every node in
	it) means everything travelling the same gap is spread across parallel
	tracks instead of collapsing onto one line.
	*/
	const LANE_STEP = 0.8;
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

	// Enclosures are expressed as centre-and-half-extent Boxes so the same
	// orthogonal overlap test used for cards works on them unchanged.
	const groupBoxes = groups.map((group) => ({
		id: group.id,
		box: {
			x: group.x + group.w / 2,
			y: group.y + group.h / 2,
			w: group.w / 2,
			h: group.h / 2,
		},
	}));

	const xs = [...placements.values()].map((placement) => placement.x);
	const corridors = corridorsOf(placements);

	for (const edge of edgeList) {
		const id = `${edge.from}>${edge.to}`;
		const from = placements.get(edge.from);
		const to = placements.get(edge.to);
		const side = sides.get(id);
		const fromPort = portsMap.get(`${id}:from`);
		const toPort = portsMap.get(`${id}:to`);
		if (!from || !to || !side || !fromPort || !toPort) continue;

		// An edge's own endpoints aren't obstacles — it has to touch them.
		const obstacles = boxes
			.filter((entry) => entry.id !== edge.from && entry.id !== edge.to)
			.map((entry) => entry.box);

		// Nor are the rings at either end: an edge inside a ring, or leaving
		// one for the next, has to cross those two enclosures to exist.
		const enclosures = groupBoxes
			.filter((entry) => entry.id !== from.group && entry.id !== to.group)
			.map((entry) => entry.box);

		const points = routeEdge(
			fromPort,
			toPort,
			side.from,
			side.to,
			laneOf.get(id) ?? 0,
			{
				from: stubsMap.get(`${id}:from`) ?? STUB,
				to: stubsMap.get(`${id}:to`) ?? STUB,
			},
			corridors,
			obstacles,
			enclosures,
			{
				from: roomsMap.get(`${id}:from`) ?? 0,
				to: roomsMap.get(`${id}:to`) ?? 0,
			},
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
	extent is the drawn bounding box in layout units, wide enough for the ring
	enclosures and for the outside tracks a detouring edge uses. The view maps
	it onto its viewBox, so the diagram scales to whatever shape the topology
	turns out to be rather than being squeezed into a fixed space.
	*/
	const ys = [...placements.values()].map((placement) => placement.y);

	const extent =
		placements.size === 0
			? { x: 0, y: 0, w: 100, h: 100 }
			: (() => {
					const left =
						Math.min(...xs, ...groups.map((group) => group.x)) - EXTENT_PAD.x;
					const right =
						Math.max(...xs, ...groups.map((group) => group.x + group.w)) +
						EXTENT_PAD.x;
					const top =
						Math.min(...ys, ...groups.map((group) => group.y)) - EXTENT_PAD.y;
					const bottom =
						Math.max(...ys, ...groups.map((group) => group.y + group.h)) +
						EXTENT_PAD.y;

					return { x: left, y: top, w: right - left, h: bottom - top };
				})();

	return { placements, groups, edges: edgesOut, ports, extent };
};

const pathsFrom = (
	selection: DiagnosticsSelection | null,
	edges: DiagEdge[],
) => {
	// Only a stage has a flow path through it; selecting a ring highlights its
	// members instead, which the view resolves from the placements directly.
	if (selection === null || selection.kind !== "stage") {
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
			const candidates =
				direction === "up" ? incoming.get(current) : outgoing.get(current);

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

	return {
		upstream: walk(selection.name, "up"),
		downstream: walk(selection.name, "down"),
	};
};

type StageState = "live" | "stale" | "unseen";

const stageState = (stage: NodeStats | undefined, atNs: number): StageState => {
	if (stage === undefined) return "unseen";
	if (atNs - stage.lastAtNs <= TOPOLOGY_LIVE_WINDOW_NS) return "live";
	return "stale";
};

const STAGE_TONE: Record<
	StageState,
	{ dot: string; borderColor: string; text: string }
> = {
	live: { dot: "bg-(--up)", borderColor: "var(--up)", text: "text-(--up)" },
	stale: { dot: "bg-(--f4)", borderColor: "var(--f4)", text: "text-(--f4)" },
	unseen: {
		dot: "bg-(--line2)",
		borderColor: "var(--line)",
		text: "text-(--f4)",
	},
};

export type BacklogTone = "clear" | "building" | "backed-up";

/*
backlogTone reads current pressure against this stage's own session peak — a
ring's absolute capacity isn't known client-side, but "close to the worst
this stage has ever seen" is the same signal the old queue tanks' high-water
mark gave, and needs no configuration.
*/
export const backlogTone = (
	backlog: number,
	maxBacklog: number,
): BacklogTone => {
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

/*
shortLabel drops the ring's own name from a card sitting inside that ring's
enclosure, which already carries it. Stage labels are namespaced by their ring
("ticker.pumpdump"), so repeating the prefix on every card costs the width the
part that actually distinguishes them needs. The full label stays as the
card's title attribute and as its identity everywhere else.
*/
const shortLabel = (placement: Placement): string =>
	placement.group !== "" && placement.label.startsWith(`${placement.group}.`)
		? placement.label.slice(placement.group.length + 1)
		: placement.label;

/*
GroupHull draws one runtime ring as an enclosure around the stages that run in
it, titled with the ring's own name. This is the part of the diagram that
cannot be derived from a trace of hops: the ring reported its own membership
(see runtime.Composed), so what is drawn here is the actual composition rather
than a cluster inferred from label prefixes.
*/
const GroupHull = ({
	group,
	extent,
	selected,
	dimmed,
	onSelect,
}: {
	group: GroupBox;
	extent: Extent;
	selected: boolean;
	dimmed: boolean;
	onSelect: (selection: DiagnosticsSelection) => void;
}) => (
	<button
		type="button"
		onClick={(event) => {
			event.stopPropagation();
			onSelect({ kind: "group", name: group.id });
		}}
		aria-label={`Inspect the ${group.label} ring`}
		className={cn(
			"absolute z-0 cursor-pointer rounded-sm border border-dashed transition-all",
			selected
				? "border-(--acc) bg-(--acc)/6"
				: "border-(--line2) bg-(--f4)/4 hover:bg-(--f4)/8",
			dimmed && "opacity-25",
		)}
		style={{
			left: `${((group.x - extent.x) / extent.w) * 100}%`,
			top: `${((group.y - extent.y) / extent.h) * 100}%`,
			width: `${(group.w / extent.w) * 100}%`,
			height: `${(group.h / extent.h) * 100}%`,
		}}
	>
		{/* The title straddles the top border rather than sitting in a band
		of its own above the cards. A reserved band costs a fraction of a row
		on every ring, which across four stacked ring layers is most of a card
		of height — and the border it interrupts is what makes it read as the
		box's name rather than as loose text inside it. */}
		<span
			className={cn(
				"absolute left-2 top-0 -translate-y-1/2 whitespace-nowrap bg-(--bg) px-1 font-mono text-[9px] uppercase tracking-widest",
				selected ? "text-(--acc)" : "text-(--f4)",
			)}
		>
			{group.label}
		</span>
	</button>
);

const StageNode = ({
	placement,
	extent,
	stage,
	state,
	density,
	selected,
	dimmed,
	highlight,
	onSelect,
}: {
	placement: Placement;
	extent: Extent;
	stage: NodeStats | undefined;
	state: StageState;
	density: NodeDensity;
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
	const type = NODE_TYPE[density];

	return (
		<button
			type="button"
			onClick={(event) => {
				event.stopPropagation();
				onSelect({ kind: "stage", name: placement.id });
			}}
			aria-label={`Inspect ${placement.label}`}
			className={cn(
				"diag-node absolute z-10 -translate-x-1/2 -translate-y-1/2 cursor-pointer overflow-hidden rounded-xs border bg-(--surface) text-left transition-all hover:bg-(--raised)",
				// Tight leading is what buys the breathing room: at the default
				// line-height a third of a small card is the space above and
				// below the digits rather than the digits themselves.
				"flex flex-col justify-center leading-[1.15]",
				type.pad,
				type.gap,
				state === "live" && !dimmed && "diag-node-live",
				selected &&
					"outline outline-(--acc) outline-offset-1 ring-1 ring-(--acc)/40",
				highlight === "up" &&
					!selected &&
					"outline outline-(--warn)/70 outline-offset-1",
				highlight === "down" &&
					!selected &&
					"outline outline-(--info)/70 outline-offset-1",
				dimmed && "opacity-20",
			)}
			style={{
				left: `${toPercent(placement, extent).left}%`,
				top: `${toPercent(placement, extent).top}%`,
				// A share of the extent the surface was sized against, so the
				// card tracks the diagram exactly; how much of the card's
				// content survives at that size is `density`'s job.
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
				style={{
					height: `${fillRatio * 100}%`,
					opacity: backlog > 0 ? 0.16 : 0,
				}}
			/>
			<div className="relative flex items-center gap-1">
				<span className={`size-1.5 shrink-0 rounded-full ${tone.dot}`} />
				<span
					className={cn(
						"truncate font-mono font-semibold uppercase tracking-wide text-(--f1)",
						type.title,
					)}
					title={placement.label}
				>
					{shortLabel(placement)}
				</span>
			</div>
			{/* Columns are sized in fr rather than fixed ch so a long value
			borrows room from its neighbours instead of being clipped — the
			fixed 3ch/7ch/6ch grid truncated real readings like "peak 1.5K". */}
			{density !== "name" ? (
				<div className="relative grid grid-cols-[auto_1fr_auto] items-baseline gap-x-1.5 font-mono">
					<span className={cn("uppercase text-(--f4)", type.meta)}>rate</span>
					<span
						className={cn(
							"text-right font-bold tabular-nums text-(--acc)",
							type.rate,
							stage === undefined && "text-(--f4)",
						)}
					>
						{stage === undefined ? "—" : formatRate(stage.avgGapNs)}
					</span>
					<span className={cn("text-right uppercase", type.meta, tone.text)}>
						{state}
					</span>
				</div>
			) : null}
			{density === "full" || density === "compact" ? (
				<div
					className={cn(
						"relative grid grid-cols-[auto_1fr_auto] items-baseline gap-x-1.5 font-mono text-(--f3)",
						type.meta,
					)}
				>
					<span className="text-(--f4)">last</span>
					<span className="text-right tabular-nums">
						{formatNanos(stage?.lastGapNs)}
					</span>
					<span className="text-right tabular-nums">
						{stage !== undefined
							? `${formatCount(stage.seqCount)} ops`
							: "unseen"}
					</span>
				</div>
			) : null}
			{density === "full" ? (
				<div
					className={cn(
						"relative grid grid-cols-[auto_1fr_auto] items-baseline gap-x-1.5 font-mono text-(--f3)",
						type.meta,
					)}
				>
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
					<span className="text-right tabular-nums">
						peak {formatCount(maxBacklog)}
					</span>
				</div>
			) : null}
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
		<g
			onMouseEnter={() => onHover(true)}
			onMouseLeave={() => onHover(false)}
			className="cursor-pointer"
		>
			<title>{`${edge.from} → ${edge.to}`}</title>
			<path
				d={edge.d}
				fill="none"
				stroke="transparent"
				strokeWidth={6}
				vectorEffect="non-scaling-stroke"
			/>
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
}: {
	edge: DiagEdge;
	extent: Extent;
	dimmed: boolean;
	hovered: boolean;
}) => {
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

/*
useMeasuredPanel reports the panel's own pixel size. The layout needs it
because the diagram is packed to fit this space rather than to a fixed width,
and a ResizeObserver is what makes that hold through a window resize or the
detail rail changing width — not just on first paint.
*/
const useMeasuredPanel = () => {
	const ref = useRef<HTMLDivElement | null>(null);
	const [size, setSize] = useState<{ w: number; h: number } | null>(null);

	useEffect(() => {
		const element = ref.current;

		if (element === null) return;

		const observer = new ResizeObserver((entries) => {
			const box = entries[0]?.contentRect;

			if (box === undefined || box.width <= 0 || box.height <= 0) return;

			// Ignore sub-pixel churn: this drives a relayout, and an observer
			// that fires on every fractional change during a resize drag would
			// rebuild the graph dozens of times a second for no visible gain.
			setSize((current) =>
				current !== null &&
				Math.abs(current.w - box.width) < 4 &&
				Math.abs(current.h - box.height) < 4
					? current
					: { w: box.width, h: box.height },
			);
		});

		observer.observe(element);

		return () => observer.disconnect();
	}, []);

	return { ref, size };
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
export const DiagnosticsGraph = ({
	nodes,
	edges,
	atNs,
	selection,
	onSelect,
}: DiagnosticsGraphProps) => {
	const { ref: panelRef, size: panel } = useMeasuredPanel();
	const [hoveredEdgeId, setHoveredEdgeId] = useState<string | null>(null);

	/*
	The layout is packed to fit the panel, so the panel has to be measured
	before it can be laid out. Its size is quantised first: it drives a search
	over packing widths, and re-running that per pixel would rebuild the whole
	diagram continuously while a window edge is being dragged.
	*/
	const target = useMemo(
		() =>
			panel === null
				? DEFAULT_PANEL
				: {
						w: Math.round(panel.w / 16) * 16,
						h: Math.round(panel.h / 16) * 16,
					},
		[panel],
	);

	// nodes/edges are Maps mutated in place by topologyStore.ingest, so their
	// references never change — atNs (the freshest stamp timestamp seen) is
	// the actual "this data changed" signal the memo needs to depend on.
	// biome-ignore lint/correctness/useExhaustiveDependencies: nodes/edges are read for their mutated contents, not their (stable) reference — atNs is the real change signal.
	const graph = useMemo(
		() => buildDiagnosticsGraph(nodes, edges, target),
		[atNs, target],
	);

	const { upstream, downstream } = useMemo(
		() => pathsFrom(selection, graph.edges),
		[selection, graph.edges],
	);

	/*
	Members of a selected ring stay lit while everything else dims — the ring
	equivalent of a stage's upstream/downstream trace.
	*/
	const members = useMemo(
		() =>
			selection?.kind === "group"
				? new Set(
						[...graph.placements.values()]
							.filter((placement) => placement.group === selection.name)
							.map((placement) => placement.id),
					)
				: new Set<string>(),
		[selection, graph.placements],
	);

	/*
	The diagram fills the panel. It only grows past it — and so only scrolls —
	when even MIN_NODE_PX cannot be met, which the clustered layout makes rare
	because it packs to this panel's shape rather than to a fixed width. The
	previous surface sized every card at a comfortable 150x92 unconditionally,
	which is why the live topology ran several screens tall and the ingress
	stages were never on screen with the strategy stages they feed.
	*/
	const surface =
		panel === null
			? null
			: {
					width: Math.max(
						panel.w,
						(graph.extent.w / (HALF.w * 2)) * MIN_NODE_PX.w,
					),
					height: Math.max(
						panel.h,
						(graph.extent.h / (HALF.h * 2)) * MIN_NODE_PX.h,
					),
				};

	// Before the first measure the layout was packed for DEFAULT_PANEL, so the
	// density has to be read against that same assumption — reading it against
	// the bare minimum instead would render one frame of name-only cards on
	// every mount, and would leave a server-rendered graph stripped for good.
	const rendered = surface ?? { width: target.w, height: target.h };
	const density = densityFor(((HALF.h * 2) / graph.extent.h) * rendered.height);

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
		<div
			ref={panelRef}
			className="relative h-full w-full overflow-auto select-none"
			onClick={() => onSelect(null)}
		>
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
			{/* The sized surface: the panel itself, unless the topology has
			grown past what MIN_NODE_PX allows in it. Before the first measure
			it simply fills the panel, so nothing flashes at a wrong size. */}
			<div
				className="relative"
				style={
					surface === null
						? { width: "100%", height: "100%" }
						: {
								width: `${Math.round(surface.width)}px`,
								height: `${Math.round(surface.height)}px`,
							}
				}
			>
				{graph.groups.map((group) => (
					<GroupHull
						key={`group:${group.id}`}
						group={group}
						extent={graph.extent}
						selected={
							selection?.kind === "group" && selection.name === group.id
						}
						dimmed={
							selection !== null &&
							!(selection.kind === "group" && selection.name === group.id)
						}
						onSelect={onSelect}
					/>
				))}
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
							const flowing =
								atNs - edge.stats.lastAtNs <= TOPOLOGY_LIVE_WINDOW_NS;
							const highlightUp =
								selection !== null &&
								(upstream.has(edge.from) || upstream.has(edge.to));
							const highlightDown =
								selection !== null &&
								(downstream.has(edge.from) || downstream.has(edge.to));
							const highlight = highlightUp
								? ("up" as const)
								: highlightDown
									? ("down" as const)
									: null;
							// A wire belongs to a selected ring only when both ends
							// do; one leaving it is that ring's boundary, not its
							// internal wiring.
							const inRing = members.has(edge.from) && members.has(edge.to);
							const dimmed =
								selection !== null && highlight === null && !inRing;
							const hovered = hoveredEdgeId === edge.id;

							return (
								<EdgePath
									key={edge.id}
									edge={edge}
									flowing={flowing}
									dimmed={dimmed}
									highlight={highlight}
									hovered={hovered}
									onHover={(isHovered) =>
										setHoveredEdgeId(isHovered ? edge.id : null)
									}
								/>
							);
						})}
					</g>
				</svg>

				{graph.edges.map((edge) => {
					const connected =
						(selection?.kind === "stage" &&
							(selection.name === edge.from || selection.name === edge.to)) ||
						(members.has(edge.from) && members.has(edge.to)) ||
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
					const selectedHere =
						selection?.kind === "stage" && selection.name === placement.id;
					const isUp = upstream.has(placement.id);
					const isDown = downstream.has(placement.id);
					const highlight = selectedHere
						? null
						: isUp
							? ("up" as const)
							: isDown
								? ("down" as const)
								: null;
					const dimmed =
						selection !== null &&
						!selectedHere &&
						highlight === null &&
						!members.has(placement.id);
					const stage = nodes.get(placement.id);

					return (
						<StageNode
							key={placement.id}
							placement={placement}
							extent={graph.extent}
							stage={stage}
							state={stageState(stage, atNs)}
							density={density}
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
