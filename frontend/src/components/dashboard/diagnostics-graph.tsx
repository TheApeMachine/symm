import { useMemo } from "react";
import type {
	ClockSnapshot,
	DiagnosticsFrame,
	ErrorSnapshot,
	QueueSnapshot,
} from "#/collections/types";
import { cn } from "@/lib/utils";

/*
DiagnosticsSelection identifies either a stage (a processing node) or a queue
(a storage node) shown on the wiring graph. The detail rail keys off the same
discriminated union.
*/
export type DiagnosticsSelection =
	| { kind: "stage"; name: string }
	| { kind: "queue"; name: string };

type NodeKind = "stage" | "queue";

/*
NODE_POS is the fixed wiring order of the analytical pipeline, read top to
bottom. Positions are percent coordinates matching the overlay svg's 0..100
viewBox. Sources sit at the top, signals fan into two rows in the middle fan-
out band, and terminals sit at the bottom. Lateral positions spread each band
across the full width so cards stay wide and text stays legible.
*/
const NODE_POS: Record<string, { x: number; y: number }> = {
	// Sources.
	crypto: { x: 32, y: 6 },
	"websocket-api": { x: 68, y: 6 },
	// Ingress rails (queues).
	"ingress.tickers": { x: 22, y: 15 },
	"ingress.trades": { x: 50, y: 15 },
	"ingress.level3": { x: 78, y: 15 },
	// Signal band — two rows of five so each card keeps breathing room.
	correlation: { x: 10, y: 23 },
	cvd: { x: 30, y: 23 },
	depthflow: { x: 50, y: 23 },
	exhaustion: { x: 70, y: 23 },
	hawkes: { x: 90, y: 23 },
	leadlag: { x: 10, y: 33 },
	liquidity: { x: 30, y: 33 },
	pumpdump: { x: 50, y: 33 },
	sentiment: { x: 70, y: 33 },
	toxicity: { x: 90, y: 33 },
	// Measurement rail (queue).
	measurements: { x: 50, y: 42 },
	// Logic band.
	category: { x: 12, y: 51 },
	manifold: { x: 29, y: 51 },
	causal: { x: 46, y: 51 },
	cognition: { x: 63, y: 51 },
	graph: { x: 78, y: 51 },
	resonance: { x: 91, y: 51 },
	// Derived rails (queues).
	"derived.category": { x: 17, y: 59 },
	"derived.causal": { x: 35, y: 59 },
	"derived.cognition": { x: 53, y: 59 },
	"derived.graph": { x: 71, y: 59 },
	"derived.resonance": { x: 89, y: 59 },
	// Strategy band.
	planner: { x: 30, y: 67 },
	mcts: { x: 50, y: 67 },
	allocation: { x: 70, y: 67 },
	// Decision + broker rails (queues).
	decisions: { x: 25, y: 75 },
	"desk.ticker": { x: 55, y: 75 },
	"desk.executions": { x: 82, y: 75 },
	// Desk + output rails.
	desk: { x: 40, y: 83 },
	positions: { x: 62, y: 83 },
	"ui.dashboard": { x: 78, y: 83 },
	"ui.manifold": { x: 90, y: 83 },
	// Terminals.
	audit: { x: 25, y: 93 },
	hub: { x: 48, y: 93 },
	"webrtc-hub": { x: 68, y: 93 },
	diagnostics: { x: 88, y: 93 },
};

const NODE_LABEL: Record<string, string> = {
	crypto: "Ingress",
	"websocket-api": "WS API",
	correlation: "Correlation",
	cvd: "CVD",
	depthflow: "Depthflow",
	exhaustion: "Exhaustion",
	hawkes: "Hawkes",
	leadlag: "Lead/Lag",
	liquidity: "Liquidity",
	pumpdump: "Pump/Dump",
	sentiment: "Sentiment",
	toxicity: "Toxicity",
	category: "Category",
	manifold: "Manifold",
	causal: "Causal",
	cognition: "Cognition",
	graph: "Graph",
	resonance: "Resonance",
	planner: "Planner",
	mcts: "MCTS",
	allocation: "Allocation",
	desk: "Desk",
	audit: "Audit",
	hub: "UI hub",
	"webrtc-hub": "WebRTC hub",
	diagnostics: "Diagnostics",
	"ingress.tickers": "Tickers",
	"ingress.trades": "Trades",
	"ingress.level3": "Level 3",
	measurements: "Measurements",
	"derived.category": "Categories",
	"derived.causal": "Causal state",
	"derived.cognition": "Cognition",
	"derived.graph": "Graphs",
	"derived.resonance": "Resonance",
	decisions: "Decisions",
	positions: "Positions",
	"ui.dashboard": "UI dashboard",
	"ui.manifold": "UI manifold",
	"desk.ticker": "Desk ticker",
	"desk.executions": "Desk trades",
};

// A queue is one heartbeat of 250ms; stages older than this are called stale.
const LIVE_WINDOW_NS = 2_000_000_000;

/*
HEARTBEAT_NS is the diagnostics collection period the publisher uses. Edge
health thresholds are expressed as fractions of it rather than as free-floating
magic numbers: a handoff under a tenth of a beat is healthy, one beyond that
but still inside a beat is slight latency, and anything past a full beat is
high latency.
*/
const HEARTBEAT_NS = 250_000_000;

type HealthTone = "healthy" | "slight" | "high";

const edgeHealth = (latencyNs: number | undefined): HealthTone => {
	if (latencyNs === undefined || !Number.isFinite(latencyNs) || latencyNs <= 0) {
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

/*
EDGE_HEALTH_STROKE maps latency health to muted, low-saturation light tones.
Grey is deliberately excluded: even the inactive/idle lanes keep a healthy
muted green so the wiring never falls to an undifferentiated grey field.
*/
const EDGE_HEALTH_STROKE: Record<HealthTone, string> = {
	healthy: "hsl(140 32% 62%)",
	slight: "hsl(48 42% 61%)",
	high: "hsl(0 44% 64%)",
};

const NODE_HEALTH_STROKE: Record<HealthTone, string> = {
	healthy: "hsl(140 30% 55%)",
	slight: "hsl(48 40% 57%)",
	high: "hsl(0 42% 60%)",
};

/*
HALF is each node's half-extent in the same percent units as NODE_POS, so edge
routes can attach to the actual card borders. Stages are 12% wide, 8% tall;
queue tanks are 9.5% wide and 4.4% tall.
*/
const HALF: Record<NodeKind, { w: number; h: number }> = {
	stage: { w: 6, h: 4 },
	queue: { w: 4.75, h: 2.2 },
};

const formatCount = (count: number): string =>
	new Intl.NumberFormat("en", { notation: "compact" }).format(count);

const formatNanos = (nanos: number | undefined): string => {
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

const averageNanos = (clock?: {
	count?: number;
	total_ns?: number;
}): number => {
	if ((clock?.count ?? 0) <= 0) {
		return 0;
	}

	return (clock?.total_ns ?? 0) / (clock?.count ?? 1);
};

type Placement = {
	id: string;
	kind: NodeKind;
	label: string;
	x: number;
	y: number;
};

type EdgeKind = "write" | "read" | "hop";

type Point = {
	x: number;
	y: number;
};

type NodeSide = "top" | "right" | "bottom" | "left";

type DiagEdge = {
	id: string;
	from: string;
	to: string;
	kind: EdgeKind;
	d: string;
	points: Point[];
	labelPoint: Point;
	latencyNs?: number;
	queueName?: string;
};

type RawEdge = {
	id: string;
	from: string;
	to: string;
	kind: EdgeKind;
	queueName?: string;
	latencyNs?: number;
};

type StageState = "error" | "running" | "live" | "stale" | "unseen";

const stageState = (
	name: string,
	stage: ClockSnapshot | undefined,
	errors: ErrorSnapshot[],
	atNs: number,
): StageState => {
	if (errors.some((error) => error.source === name)) {
		return "error";
	}

	if ((stage?.active ?? 0) > 0) {
		return "running";
	}

	if ((stage?.count ?? 0) === 0) {
		return "unseen";
	}

	const lastAtNs = stage?.last_at_ns ?? 0;

	if (lastAtNs > 0 && atNs - lastAtNs <= LIVE_WINDOW_NS) {
		return "live";
	}

	return "stale";
};

/*
STAGE_TONE maps a stage's health state to its border/dot/text tone. Border
colors use the same muted health palette as the edges (healthy green, slightly
amber latency, high red latency) so a node border reads as the health of the
component it represents. Only the unseen neutral is allowed off-palette.
*/
const STAGE_TONE: Record<
	StageState,
	{ dot: string; borderColor: string; text: string }
> = {
	error: {
		dot: "bg-(--down)",
		borderColor: NODE_HEALTH_STROKE.high,
		text: "text-(--down)",
	},
	live: {
		dot: "bg-(--up)",
		borderColor: NODE_HEALTH_STROKE.healthy,
		text: "text-(--up)",
	},
	running: {
		dot: "bg-(--info)",
		borderColor: NODE_HEALTH_STROKE.healthy,
		text: "text-(--info)",
	},
	stale: {
		dot: "bg-(--warn)",
		borderColor: NODE_HEALTH_STROKE.slight,
		text: "text-(--warn)",
	},
	unseen: {
		dot: "bg-(--line2)",
		borderColor: "hsl(220 10% 52%)",
		text: "text-(--f4)",
	},
};

const STATE_LABEL: Record<StageState, string> = {
	error: "error",
	running: "running",
	live: "live",
	stale: "stale",
	unseen: "unseen",
};

/*
QUEUE_IDS lists every pipeline buffer that renders as a tank. Most are
dotted wire names; a few (measurements, decisions, positions) are simple names
that still describe queues, so the kind must be declared rather than inferred
from the name.
*/
const QUEUE_IDS = new Set([
	"ingress.tickers",
	"ingress.trades",
	"ingress.level3",
	"measurements",
	"derived.category",
	"derived.causal",
	"derived.cognition",
	"derived.graph",
	"derived.resonance",
	"decisions",
	"positions",
	"desk.ticker",
	"desk.executions",
	"ui.dashboard",
	"ui.manifold",
]);

const placementsOf = (): Map<string, Placement> => {
	const placements = new Map<string, Placement>();

	for (const [id, position] of Object.entries(NODE_POS)) {
		const kind: NodeKind = QUEUE_IDS.has(id) ? "queue" : "stage";

		placements.set(id, {
			id,
			kind,
			label: NODE_LABEL[id] ?? id,
			x: position.x,
			y: position.y,
		});
	}

	return placements;
};

type Bounds = {
	left: number;
	right: number;
	top: number;
	bottom: number;
};

const ROUTE_CLEARANCE = 0.55;
const ROUTE_STUB = 0.8;
const PORT_INSET = 0.8;

/*
Routing costs make crossings more expensive than any plausible canvas detour.
Bend cost only breaks ties between otherwise equivalent clear routes.
LANE_SPACING keeps parallel same-direction segments a visible distance apart
so two edges leaving one node never run length-wise on top of each other, and
ROUTE_OVERLAP_COST_MASSIVE is a strong finite cost (not Infinity) that
dominates detours so parallel lanes separate cleanly while a trivial corner
graze stays cheaper than an absurd full-canvas detour.
*/
const ROUTE_CROSSING_COST = 120;
const ROUTE_OVERLAP_COST_MASSIVE = 900;
const ROUTE_BEND_COST = 0.35;
const LANE_SPACING = 0.6;
const ROUTE_NEAR_COST = 90;

export const placementBounds = (
	placement: Placement,
	clearance = 0,
): Bounds => ({
	left: placement.x - HALF[placement.kind].w - clearance,
	right: placement.x + HALF[placement.kind].w + clearance,
	top: placement.y - HALF[placement.kind].h - clearance,
	bottom: placement.y + HALF[placement.kind].h + clearance,
});

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
	const half = HALF[placement.kind];
	const horizontal = side === "top" || side === "bottom";
	const extent = horizontal ? half.w : half.h;
	const usable = Math.max(0, extent - PORT_INSET);
	const offset = count === 1 ? 0 : -usable + (2 * usable * index) / (count - 1);

	if (side === "top") {
		return { x: placement.x + offset, y: placement.y - half.h };
	}

	if (side === "bottom") {
		return { x: placement.x + offset, y: placement.y + half.h };
	}

	if (side === "left") {
		return { x: placement.x - half.w, y: placement.y + offset };
	}

	return { x: placement.x + half.w, y: placement.y + offset };
};

const stubPoint = (point: Point, side: NodeSide): Point => {
	if (side === "top") {
		return { x: point.x, y: point.y - ROUTE_STUB };
	}

	if (side === "bottom") {
		return { x: point.x, y: point.y + ROUTE_STUB };
	}

	if (side === "left") {
		return { x: point.x - ROUTE_STUB, y: point.y };
	}

	return { x: point.x + ROUTE_STUB, y: point.y };
};

const samePoint = (first: Point, second: Point): boolean =>
	first.x === second.x && first.y === second.y;

const simplifyPoints = (points: Point[]): Point[] => {
	const unique = points.filter(
		(point, index) => index === 0 || !samePoint(point, points[index - 1]),
	);
	const simplified: Point[] = [];

	for (const point of unique) {
		const previous = simplified.at(-1);
		const beforePrevious = simplified.at(-2);

		if (previous === undefined || beforePrevious === undefined) {
			simplified.push(point);
			continue;
		}

		const sameX = beforePrevious.x === previous.x && previous.x === point.x;
		const sameY = beforePrevious.y === previous.y && previous.y === point.y;

		if (sameX || sameY) {
			simplified[simplified.length - 1] = point;
			continue;
		}

		simplified.push(point);
	}

	return simplified;
};

const segmentBlocked = (from: Point, to: Point, bounds: Bounds): boolean => {
	if (from.y === to.y) {
		const left = Math.min(from.x, to.x);
		const right = Math.max(from.x, to.x);

		return (
			from.y > bounds.top &&
			from.y < bounds.bottom &&
			right > bounds.left &&
			left < bounds.right
		);
	}

	if (from.x === to.x) {
		const top = Math.min(from.y, to.y);
		const bottom = Math.max(from.y, to.y);

		return (
			from.x > bounds.left &&
			from.x < bounds.right &&
			bottom > bounds.top &&
			top < bounds.bottom
		);
	}

	return true;
};

export const routeIntersectsPlacement = (
	points: Point[],
	placement: Placement,
): boolean => {
	const bounds = placementBounds(placement, ROUTE_CLEARANCE);

	return points.some((point, index) => {
		if (index === 0) {
			return false;
		}

		return segmentBlocked(points[index - 1], point, bounds);
	});
};

const segmentLength = (from: Point, to: Point): number =>
	Math.abs(to.x - from.x) + Math.abs(to.y - from.y);

/*
parallelNearMiss reports whether two axis-aligned segments run in the same
direction within LANE_SPACING of each other while their spans overlap. This is
what separates two edges sharing an exit direction without forbidding clean
perpendicular crossings, which the user wants to keep allowing.
*/
const parallelNearMiss = (
	from: Point,
	to: Point,
	otherFrom: Point,
	otherTo: Point,
): boolean => {
	const horizontal = from.y === to.y;
	const otherHorizontal = otherFrom.y === otherTo.y;

	if (horizontal !== otherHorizontal) {
		return false;
	}

	if (horizontal) {
		const gap = Math.abs(from.y - otherFrom.y);

		if (gap === 0 || gap >= LANE_SPACING) {
			return false;
		}

		const firstStart = Math.min(from.x, to.x);
		const firstEnd = Math.max(from.x, to.x);
		const secondStart = Math.min(otherFrom.x, otherTo.x);
		const secondEnd = Math.max(otherFrom.x, otherTo.x);

		return Math.min(firstEnd, secondEnd) > Math.max(firstStart, secondStart);
	}

	const gap = Math.abs(from.x - otherFrom.x);

	if (gap === 0 || gap >= LANE_SPACING) {
		return false;
	}

	const firstStart = Math.min(from.y, to.y);
	const firstEnd = Math.max(from.y, to.y);
	const secondStart = Math.min(otherFrom.y, otherTo.y);
	const secondEnd = Math.max(otherFrom.y, otherTo.y);

	return Math.min(firstEnd, secondEnd) > Math.max(firstStart, secondStart);
};

const segmentOverlap = (
	from: Point,
	to: Point,
	otherFrom: Point,
	otherTo: Point,
): { crossing: boolean; overlap: number } => {
	const horizontal = from.y === to.y;
	const otherHorizontal = otherFrom.y === otherTo.y;

	if (horizontal !== otherHorizontal) {
		const horizontalFrom = horizontal ? from : otherFrom;
		const horizontalTo = horizontal ? to : otherTo;
		const verticalFrom = horizontal ? otherFrom : from;
		const verticalTo = horizontal ? otherTo : to;
		const crossX = verticalFrom.x;
		const crossY = horizontalFrom.y;
		const crosses =
			crossX > Math.min(horizontalFrom.x, horizontalTo.x) &&
			crossX < Math.max(horizontalFrom.x, horizontalTo.x) &&
			crossY > Math.min(verticalFrom.y, verticalTo.y) &&
			crossY < Math.max(verticalFrom.y, verticalTo.y);

		return { crossing: crosses, overlap: 0 };
	}

	if (horizontal && from.y !== otherFrom.y) {
		return { crossing: false, overlap: 0 };
	}

	if (!horizontal && from.x !== otherFrom.x) {
		return { crossing: false, overlap: 0 };
	}

	const firstStart = horizontal
		? Math.min(from.x, to.x)
		: Math.min(from.y, to.y);
	const firstEnd = horizontal ? Math.max(from.x, to.x) : Math.max(from.y, to.y);
	const secondStart = horizontal
		? Math.min(otherFrom.x, otherTo.x)
		: Math.min(otherFrom.y, otherTo.y);
	const secondEnd = horizontal
		? Math.max(otherFrom.x, otherTo.x)
		: Math.max(otherFrom.y, otherTo.y);

	return {
		crossing: false,
		overlap: Math.max(
			0,
			Math.min(firstEnd, secondEnd) - Math.max(firstStart, secondStart),
		),
	};
};

type RouteCost = {
	score: number;
	lanePenalties: number;
};

type TaggedSegment = {
	from: Point;
	to: Point;
	source: string;
};

const routeScore = (
	points: Point[],
	obstacles: Placement[],
	existingSegments: TaggedSegment[],
): RouteCost => {
	if (
		obstacles.some((placement) => routeIntersectsPlacement(points, placement))
	) {
		return { score: Number.POSITIVE_INFINITY, lanePenalties: 0 };
	}

	let score = 0;
	let lanePenalties = 0;

	for (let index = 1; index < points.length; index++) {
		const from = points[index - 1];
		const to = points[index];
		score += segmentLength(from, to);

		for (const existing of existingSegments) {
			const otherFrom = existing.from;
			const otherTo = existing.to;
			const relation = segmentOverlap(from, to, otherFrom, otherTo);

			if (relation.crossing) {
				score += ROUTE_CROSSING_COST;
			}

			if (relation.overlap > 0) {
				/*
				Length-wise overlap carries a dominating but finite cost:
				crossings stay allowed, but running on top of another lane is
				avoided whenever the router has any reasonable detour. Finite
				keeps a genuinely constrained edge from degrading into a
				through-node route.
				*/
				score += relation.overlap * ROUTE_OVERLAP_COST_MASSIVE;
				lanePenalties++;
			}

			if (parallelNearMiss(from, to, otherFrom, otherTo)) {
				score += ROUTE_NEAR_COST;
				lanePenalties++;
			}
		}
	}

	return {
		score: score + Math.max(0, points.length - 2) * ROUTE_BEND_COST,
		lanePenalties,
	};
};

const labelPointOf = (points: Point[]): Point => {
	let longest: [Point, Point] = [points[0], points[1]];

	for (let index = 2; index < points.length; index++) {
		const segment: [Point, Point] = [points[index - 1], points[index]];

		if (segmentLength(...segment) > segmentLength(...longest)) {
			longest = segment;
		}
	}

	return {
		x: (longest[0].x + longest[1].x) / 2,
		y: (longest[0].y + longest[1].y) / 2,
	};
};

const pathOf = (points: Point[]): string =>
	points
		.map(
			(point, index) =>
				`${index === 0 ? "M" : "L"} ${point.x.toFixed(3)} ${point.y.toFixed(3)}`,
		)
		.join(" ");

const routeEdge = (
	from: Placement,
	to: Placement,
	fromPort: Point,
	toPort: Point,
	fromSide: NodeSide,
	toSide: NodeSide,
	placements: Map<string, Placement>,
	existingSegments: TaggedSegment[],
): Point[] => {
	const start = stubPoint(fromPort, fromSide);
	const end = stubPoint(toPort, toSide);
	const obstacles = Array.from(placements.values()).filter(
		(placement) => placement.id !== from.id && placement.id !== to.id,
	);
	const horizontalChannels = new Set<number>([1, 99, start.y, end.y]);
	const verticalChannels = new Set<number>([1, 99, start.x, end.x]);

	for (const obstacle of obstacles) {
		const bounds = placementBounds(obstacle, ROUTE_CLEARANCE);
		horizontalChannels.add(bounds.top);
		horizontalChannels.add(bounds.bottom);
		verticalChannels.add(bounds.left);
		verticalChannels.add(bounds.right);
	}

	/*
	Existing lanes seed the channel set from both directions so a parallel edge
	has an explicit offset lane to choose instead of sharing a lane and paying
	the overlap cost on the only candidate that exists. The offset lanes sit one
	LANE_SPACING off the occupied lane, which the near-miss penalty then keeps
	clear for opposite-direction or crossing traffic.
	*/
	for (const existing of existingSegments) {
		const segmentFrom = existing.from;
		const segmentTo = existing.to;

		if (segmentFrom.y === segmentTo.y) {
			horizontalChannels.add(segmentFrom.y - LANE_SPACING);
			horizontalChannels.add(segmentFrom.y + LANE_SPACING);
			verticalChannels.add(segmentFrom.x);
			verticalChannels.add(segmentTo.x);

			continue;
		}

		if (segmentFrom.x === segmentTo.x) {
			verticalChannels.add(segmentFrom.x - LANE_SPACING);
			verticalChannels.add(segmentFrom.x + LANE_SPACING);
			horizontalChannels.add(segmentFrom.y);
			horizontalChannels.add(segmentTo.y);
		}
	}

	const candidates: Point[][] = [
		[fromPort, start, { x: start.x, y: end.y }, end, toPort],
		[fromPort, start, { x: end.x, y: start.y }, end, toPort],
	];

	for (const y of horizontalChannels) {
		candidates.push([
			fromPort,
			start,
			{ x: start.x, y },
			{ x: end.x, y },
			end,
			toPort,
		]);
	}

	for (const x of verticalChannels) {
		candidates.push([
			fromPort,
			start,
			{ x, y: start.y },
			{ x, y: end.y },
			end,
			toPort,
		]);
	}

	let best = simplifyPoints(candidates[0]);
	let bestCost = routeScore(best, obstacles, existingSegments);

	for (const candidate of candidates) {
		const points = simplifyPoints(candidate);
		const cost = routeScore(points, obstacles, existingSegments);

		if (
			cost.score < bestCost.score ||
			(cost.score === bestCost.score &&
				cost.lanePenalties < bestCost.lanePenalties)
		) {
			best = points;
			bestCost = cost;
		}
	}

	return best;
};

type EdgeActivity = "flowing" | "held" | "idle";

const edgeActivity = (
	edge: DiagEdge,
	queue: QueueSnapshot | undefined,
	queueDeltas: Map<string, number>,
	hopDeltas: Map<string, number>,
): EdgeActivity => {
	if (edge.kind === "hop") {
		return (hopDeltas.get(`${edge.from}>${edge.to}`) ?? 0) > 0
			? "flowing"
			: "idle";
	}

	const depth = queue?.depth ?? 0;
	const delta = edge.queueName ? (queueDeltas.get(edge.queueName) ?? 0) : 0;

	if (edge.kind === "write") {
		if (delta > 0) {
			return "flowing";
		}

		return depth > 0 ? "held" : "idle";
	}

	if (delta < 0) {
		return "flowing";
	}

	return depth > 0 ? "held" : "idle";
};

type RoutingCache = {
	signature: string[];
	edges: DiagEdge[];
};

// Queue depths and timing counters change every frame, while topology almost
// never does. Keep exactly one routed topology so telemetry updates do not pay
// the obstacle router's cost or grow an unbounded cache.
let routingCache: RoutingCache | undefined;

const cachedRouting = (raw: RawEdge[]): DiagEdge[] | undefined => {
	if (
		routingCache === undefined ||
		routingCache.signature.length !== raw.length ||
		raw.some((edge, index) => routingCache?.signature[index] !== edge.id)
	) {
		return undefined;
	}

	const telemetry = new Map(raw.map((edge) => [edge.id, edge]));

	return routingCache.edges.map((edge) => ({
		...edge,
		latencyNs: telemetry.get(edge.id)?.latencyNs,
		queueName: telemetry.get(edge.id)?.queueName,
	}));
};

export const buildDiagnosticsGraph = (frame: DiagnosticsFrame) => {
	const placements = placementsOf();
	const queuesByName = new Map(
		(frame.queues ?? []).map((queue) => [queue.name, queue]),
	);
	const raw: RawEdge[] = [];

	for (const queue of frame.queues ?? []) {
		if (placements.get(queue.name) === undefined) {
			continue;
		}

		for (const writer of queue.writers) {
			if (placements.get(writer) === undefined) {
				continue;
			}

			raw.push({
				id: `write:${writer}>${queue.name}`,
				from: writer,
				to: queue.name,
				kind: "write",
				queueName: queue.name,
			});
		}

		for (const reader of queue.readers) {
			if (placements.get(reader) === undefined) {
				continue;
			}

			raw.push({
				id: `read:${queue.name}>${reader}`,
				from: queue.name,
				to: reader,
				kind: "read",
				queueName: queue.name,
			});
		}
	}

	for (const hop of frame.hops ?? []) {
		if (
			placements.get(hop.from) === undefined ||
			placements.get(hop.to) === undefined
		) {
			continue;
		}

		raw.push({
			id: `hop:${hop.from}>${hop.to}`,
			from: hop.from,
			to: hop.to,
			kind: "hop",
			latencyNs: averageNanos(hop),
		});
	}

	const cachedEdges = cachedRouting(raw);

	if (cachedEdges !== undefined) {
		return { placements, queuesByName, edges: cachedEdges };
	}

	type Attachment = {
		edge: RawEdge;
		end: "from" | "to";
		side: NodeSide;
		opposite: number;
	};

	const sides = new Map<string, { from: NodeSide; to: NodeSide }>();
	const attachments = new Map<string, Attachment[]>();

	for (const edge of raw) {
		const from = placements.get(edge.from);
		const to = placements.get(edge.to);

		if (from === undefined || to === undefined) {
			continue;
		}

		const edgeSides = sidesFor(from, to);
		sides.set(edge.id, edgeSides);

		const fromKey = `${edge.from}:${edgeSides.from}`;
		const toKey = `${edge.to}:${edgeSides.to}`;
		const fromAttachments = attachments.get(fromKey) ?? [];
		const toAttachments = attachments.get(toKey) ?? [];

		fromAttachments.push({
			edge,
			end: "from",
			side: edgeSides.from,
			opposite:
				edgeSides.from === "top" || edgeSides.from === "bottom" ? to.x : to.y,
		});
		toAttachments.push({
			edge,
			end: "to",
			side: edgeSides.to,
			opposite:
				edgeSides.to === "top" || edgeSides.to === "bottom" ? from.x : from.y,
		});
		attachments.set(fromKey, fromAttachments);
		attachments.set(toKey, toAttachments);
	}

	const ports = new Map<string, Point>();

	for (const group of attachments.values()) {
		group.sort(
			(first, second) =>
				first.opposite - second.opposite ||
				first.edge.id.localeCompare(second.edge.id),
		);

		group.forEach((attachment, index) => {
			const placement = placements.get(
				attachment.end === "from" ? attachment.edge.from : attachment.edge.to,
			);

			if (placement === undefined) {
				return;
			}

			ports.set(
				`${attachment.edge.id}:${attachment.end}`,
				portPoint(placement, attachment.side, index, group.length),
			);
		});
	}

	const routingOrder = [...raw].sort((first, second) => {
		const firstFrom = placements.get(first.from);
		const firstTo = placements.get(first.to);
		const secondFrom = placements.get(second.from);
		const secondTo = placements.get(second.to);

		if (
			firstFrom === undefined ||
			firstTo === undefined ||
			secondFrom === undefined ||
			secondTo === undefined
		) {
			return first.id.localeCompare(second.id);
		}

		const firstSpan =
			Math.abs(firstFrom.x - firstTo.x) + Math.abs(firstFrom.y - firstTo.y);
		const secondSpan =
			Math.abs(secondFrom.x - secondTo.x) + Math.abs(secondFrom.y - secondTo.y);

		if (first.from === second.from) {
			if (secondTo.x !== firstTo.x) {
				return secondTo.x - firstTo.x;
			}

			return secondTo.y - firstTo.y || first.id.localeCompare(second.id);
		}

		return firstSpan - secondSpan || first.id.localeCompare(second.id);
	});
	const edges: DiagEdge[] = [];
	const routedSegments: TaggedSegment[] = [];

	for (const edge of routingOrder) {
		const from = placements.get(edge.from);
		const to = placements.get(edge.to);
		const edgeSides = sides.get(edge.id);
		const fromPort = ports.get(`${edge.id}:from`);
		const toPort = ports.get(`${edge.id}:to`);

		if (
			from === undefined ||
			to === undefined ||
			edgeSides === undefined ||
			fromPort === undefined ||
			toPort === undefined
		) {
			continue;
		}

		const points = routeEdge(
			from,
			to,
			fromPort,
			toPort,
			edgeSides.from,
			edgeSides.to,
			placements,
			routedSegments,
		);

		for (let index = 1; index < points.length; index++) {
			routedSegments.push({
				from: points[index - 1],
				to: points[index],
				source: edge.from,
			});
		}

		edges.push({
			id: edge.id,
			from: edge.from,
			to: edge.to,
			kind: edge.kind,
			d: pathOf(points),
			points,
			labelPoint: labelPointOf(points),
			latencyNs: edge.latencyNs,
			queueName: edge.queueName,
		});
	}

	routingCache = {
		signature: raw.map((edge) => edge.id),
		edges,
	};

	return { placements, queuesByName, edges };
};

const pathsFrom = (
	selection: DiagnosticsSelection | null,
	edges: DiagEdge[],
) => {
	if (selection === null) {
		return { upstream: new Set<string>(), downstream: new Set<string>() };
	}

	const outgoing = new Map<string, DiagEdge[]>();
	const incoming = new Map<string, DiagEdge[]>();

	for (const edge of edges) {
		const out = outgoing.get(edge.from) ?? [];
		out.push(edge);
		outgoing.set(edge.from, out);

		const inn = incoming.get(edge.to) ?? [];
		inn.push(edge);
		incoming.set(edge.to, inn);
	}

	// "up" walks edges backwards (who feeds me); "down" walks them forwards
	// (who I feed). Both return the reachable node names minus the start node.
	const walk = (start: string, direction: "up" | "down"): Set<string> => {
		const visited = new Set<string>([start]);
		const frontier = [start];

		while (frontier.length > 0) {
			const current = frontier.shift() as string;
			const candidates =
				direction === "up" ? incoming.get(current) : outgoing.get(current);

			for (const edge of candidates ?? []) {
				const next = direction === "up" ? edge.from : edge.to;

				if (visited.has(next)) {
					continue;
				}

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

const StageNode = ({
	placement,
	stage,
	state,
	atNs,
	selected,
	dimmed,
	highlight,
	onSelect,
}: {
	placement: Placement;
	stage: ClockSnapshot | undefined;
	state: StageState;
	atNs: number;
	selected: boolean;
	dimmed: boolean;
	highlight: "up" | "down" | null;
	onSelect: (selection: DiagnosticsSelection) => void;
}) => {
	const tone = STAGE_TONE[state];
	const runningNs =
		(stage?.active ?? 0) > 0 && (stage?.started_ns ?? 0) > 0
			? Math.max(0, atNs - (stage?.started_ns ?? atNs))
			: 0;

	return (
		<button
			type="button"
			onClick={() => onSelect({ kind: "stage", name: placement.id })}
			aria-label={`Inspect ${placement.label}`}
			className={cn(
				"diag-node absolute z-10 -translate-x-1/2 -translate-y-1/2 cursor-pointer rounded-md border-2 bg-(--surface) px-2 py-1.5 text-left transition-opacity hover:bg-(--raised)",
				"flex flex-col justify-between",
				selected && "outline outline-(--acc) outline-offset-1",
				highlight === "up" && !selected && "outline outline-(--warn)/60",
				highlight === "down" && !selected && "outline outline-(--info)/60",
				dimmed && "opacity-20",
			)}
			style={{
				left: `${placement.x}%`,
				top: `${placement.y}%`,
				width: `${HALF.stage.w * 2}%`,
				height: `${HALF.stage.h * 2}%`,
				borderColor: tone.borderColor,
			}}
		>
			<div className="flex items-center gap-1">
				<span className={`size-1.5 shrink-0 rounded-full ${tone.dot}`} />
				<span className="truncate font-mono text-[9px] font-semibold uppercase tracking-wide text-(--f1)">
					{placement.label}
				</span>
			</div>
			<div className="grid grid-cols-[2.5ch_7ch_6ch] items-baseline gap-1 font-mono">
				<span className="text-[7px] uppercase text-(--f4)">
					{runningNs > 0 ? "run" : "avg"}
				</span>
				<span
					className={cn(
						"text-right text-[10px] font-bold tabular-nums text-(--acc)",
						stage === undefined && "text-(--f4)",
					)}
				>
					{stage === undefined
						? "—"
						: formatNanos(runningNs > 0 ? runningNs : averageNanos(stage))}
				</span>
				<span
					className={`truncate text-right text-[7px] uppercase ${tone.text}`}
				>
					{STATE_LABEL[state]}
				</span>
			</div>
			<div className="grid grid-cols-[2.5ch_7ch_6ch] items-baseline gap-1 font-mono text-[7px] text-(--f4)">
				<span>last</span>
				<span className="text-right tabular-nums">
					{formatNanos(stage?.last_ns)}
				</span>
				<span className="truncate text-right tabular-nums">
					{(stage?.active ?? 0) > 0
						? `${formatCount(stage?.active ?? 0)} active`
						: (stage?.count ?? 0) > 0
							? `${formatCount(stage?.count ?? 0)} ops`
							: "unseen"}
				</span>
			</div>
		</button>
	);
};

const QUEUE_TONE: Record<string, { border: string; fill: string }> = {
	ingress: {
		border: "border-(--info)/70",
		fill: "bg-(--info)",
	},
	rail: {
		border: "border-(--up)/70",
		fill: "bg-(--up)",
	},
	derived: {
		border: "border-(--acc)/70",
		fill: "bg-(--acc)",
	},
	strategy: {
		border: "border-(--warn)/70",
		fill: "bg-(--warn)",
	},
	ui: {
		border: "border-(--brand)/70",
		fill: "bg-(--brand)",
	},
	broker: {
		border: "border-(--down)/70",
		fill: "bg-(--down)",
	},
};

const QueueNode = ({
	placement,
	queue,
	delta,
	selected,
	dimmed,
	highlight,
	onSelect,
}: {
	placement: Placement;
	queue: QueueSnapshot;
	delta: number;
	selected: boolean;
	dimmed: boolean;
	highlight: "up" | "down" | null;
	onSelect: (selection: DiagnosticsSelection) => void;
}) => {
	const peak = Math.max(queue.high_water, queue.cap ?? 0, 1);
	const fill = Math.min(100, Math.round((queue.depth / peak) * 100));
	const tone = QUEUE_TONE[queue.kind] ?? QUEUE_TONE.rail;
	const deltaText =
		delta > 0
			? `+${formatCount(delta)}`
			: delta !== 0
				? formatCount(delta)
				: "";

	return (
		<button
			type="button"
			onClick={() => onSelect({ kind: "queue", name: placement.id })}
			aria-label={`Inspect queue ${queue.name}`}
			title={queue.name}
			className={cn(
				"diag-node absolute z-10 -translate-x-1/2 -translate-y-1/2 cursor-pointer text-left transition-opacity",
				selected && "rounded-md outline outline-(--acc) outline-offset-1",
				highlight === "up" &&
					!selected &&
					"rounded-md outline outline-(--warn)/60",
				highlight === "down" &&
					!selected &&
					"rounded-md outline outline-(--info)/60",
				dimmed && "opacity-20",
			)}
			style={{
				left: `${placement.x}%`,
				top: `${placement.y}%`,
				width: `${HALF.queue.w * 2}%`,
				height: `${HALF.queue.h * 2}%`,
			}}
		>
			<span
				className={cn(
					"relative block h-full w-full overflow-hidden rounded-md border bg-(--sunken)",
					tone.border,
				)}
			>
				{/* Queue depth rises from the floor of the buffer. */}
				<span
					className={cn(
						"absolute inset-x-0 bottom-0 block transition-all duration-300",
						tone.fill,
					)}
					style={{
						height: `${fill}%`,
						minHeight: queue.depth > 0 ? "2px" : 0,
						opacity: queue.depth > 0 ? 0.42 : 0,
					}}
				/>
				<span
					className={cn(
						"absolute inset-x-0 block h-px transition-all duration-300",
						tone.fill,
					)}
					style={{ bottom: `${fill}%`, opacity: queue.depth > 0 ? 0.9 : 0 }}
				/>
				<span className="absolute inset-0 grid grid-cols-[minmax(0,1fr)_7ch_6ch] items-center gap-1 px-1 font-mono">
					<span className="truncate text-[8px] uppercase tracking-wide text-(--f1)">
						{placement.label}
					</span>
					<span
						className={cn(
							"text-right text-[8px] font-bold tabular-nums text-(--f1)",
							queue.depth === 0 && "text-(--f4)",
						)}
					>
						{formatCount(queue.depth)}
					</span>
					{delta !== 0 ? (
						<span
							className={cn(
								"text-right text-[7px] tabular-nums",
								delta > 0 ? "text-(--warn)" : "text-(--up)",
							)}
						>
							{deltaText}
						</span>
					) : (
						<span aria-hidden="true" />
					)}
				</span>
			</span>
		</button>
	);
};

const EdgePath = ({
	edge,
	activity,
	queue,
	dimmed,
	highlight,
}: {
	edge: DiagEdge;
	activity: EdgeActivity;
	queue: QueueSnapshot | undefined;
	dimmed: boolean;
	highlight: "up" | "down" | null;
}) => {
	const health = edgeHealth(edge.latencyNs);
	const active = activity === "flowing";
	const stroke =
		highlight === "up"
			? "var(--warn)"
			: highlight === "down"
				? "var(--info)"
				: EDGE_HEALTH_STROKE[health];

	return (
		<path
			d={edge.d}
			data-from={edge.from}
			data-to={edge.to}
			data-kind={edge.kind}
			data-health={health}
			fill="none"
			stroke={stroke}
			strokeWidth={edge.kind === "hop" ? 1 : 1.4}
			vectorEffect="non-scaling-stroke"
			pathLength={100}
			strokeLinecap="round"
			strokeLinejoin="round"
			strokeDasharray={active ? "4 5" : undefined}
			className={cn(
				"diag-edge",
				active && "diag-flow",
				!active && "diag-solid",
				dimmed && "opacity-10",
			)}
			data-queue={queue?.name}
		/>
	);
};

const EdgeLatency = ({ edge, dimmed }: { edge: DiagEdge; dimmed: boolean }) => {
	if ((edge.latencyNs ?? 0) <= 0) {
		return null;
	}

	return (
		<div
			className={cn(
				"pointer-events-none absolute z-5 min-w-[7ch] -translate-x-1/2 -translate-y-1/2 rounded-sm border border-(--line) bg-(--bg)/90 px-1 py-px text-center font-mono text-[7px] tabular-nums text-(--f3)",
				dimmed && "opacity-15",
			)}
			style={{ left: `${edge.labelPoint.x}%`, top: `${edge.labelPoint.y}%` }}
			title={`${NODE_LABEL[edge.from] ?? edge.from} to ${NODE_LABEL[edge.to] ?? edge.to} average handoff latency`}
		>
			{formatNanos(edge.latencyNs)}
		</div>
	);
};

export type DiagnosticsGraphProps = {
	frame: DiagnosticsFrame;
	queueDeltas: Map<string, number>;
	hopDeltas: Map<string, number>;
	selection: DiagnosticsSelection | null;
	onSelect: (selection: DiagnosticsSelection) => void;
};

/*
DiagnosticsGraph renders the live analytical data plane top to bottom as a
wiring diagram. Stages are metric cards, queue depth rises inside rectangular
tanks, and edges use distributed ports and obstacle-aware orthogonal routes.
Live edges animate in the direction of flow; held edges pulse; idle edges fade
to grey. Selecting a node highlights its upstream feeders and downstream
consumers while dimming the rest of the plane.
*/
export const DiagnosticsGraph = ({
	frame,
	queueDeltas,
	hopDeltas,
	selection,
	onSelect,
}: DiagnosticsGraphProps) => {
	const graph = useMemo(() => buildDiagnosticsGraph(frame), [frame]);
	const { upstream, downstream } = useMemo(
		() => pathsFrom(selection, graph.edges),
		[selection, graph.edges],
	);
	const stagesByName = useMemo(
		() => new Map((frame.stages ?? []).map((stage) => [stage.name, stage])),
		[frame.stages],
	);
	const atNs = frame.at_ns ?? 0;

	if ((frame.queues ?? []).length === 0) {
		return (
			<div className="flex h-full items-center justify-center font-mono text-[10px] uppercase tracking-widest text-(--f4)">
				Waiting for queue topology
			</div>
		);
	}

	return (
		<div className="relative h-full w-full overflow-hidden">
			<style>{`
				@keyframes diag-dash-flow {
					from { stroke-dashoffset: 0; }
					to { stroke-dashoffset: -27; }
				}
				.diag-edge { stroke-linecap: round; }
				.diag-flow {
					stroke-dasharray: 4 5;
					animation: diag-dash-flow 0.9s linear infinite;
				}
				.diag-solid {
					stroke-dasharray: none;
					animation: none;
				}
			`}</style>
			<svg
				viewBox="0 0 100 100"
				preserveAspectRatio="none"
				className="absolute inset-0 h-full w-full"
				aria-hidden="true"
			>
				{graph.edges.map((edge) => {
					const queue = edge.queueName
						? graph.queuesByName.get(edge.queueName)
						: undefined;
					const activity = edgeActivity(edge, queue, queueDeltas, hopDeltas);
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
					const dimmed = selection !== null && highlight === null;

					return (
						<EdgePath
							key={edge.id}
							edge={edge}
							activity={activity}
							queue={queue}
							dimmed={dimmed}
							highlight={highlight}
						/>
					);
				})}
			</svg>
			{graph.edges.map((edge) => {
				const connected =
					selection?.name === edge.from ||
					selection?.name === edge.to ||
					upstream.has(edge.from) ||
					upstream.has(edge.to) ||
					downstream.has(edge.from) ||
					downstream.has(edge.to);

				return (
					<EdgeLatency
						key={`latency:${edge.id}`}
						edge={edge}
						dimmed={selection !== null && !connected}
					/>
				);
			})}
			{Array.from(graph.placements.values()).map((placement) => {
				const selectedHere = selection?.name === placement.id;
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
					selection !== null && !selectedHere && highlight === null;

				if (placement.kind === "queue") {
					const queue = graph.queuesByName.get(placement.id) ?? {
						name: placement.id,
						kind: "rail",
						writers: [],
						readers: [],
						depth: 0,
						cap: 0,
						high_water: 0,
					};
					const delta = queueDeltas.get(placement.id) ?? 0;

					return (
						<QueueNode
							key={placement.id}
							placement={placement}
							queue={queue}
							delta={delta}
							selected={selectedHere}
							dimmed={dimmed}
							highlight={highlight}
							onSelect={onSelect}
						/>
					);
				}

				const stage = stagesByName.get(placement.id);

				return (
					<StageNode
						key={placement.id}
						placement={placement}
						stage={stage}
						state={stageState(placement.id, stage, frame.errors ?? [], atNs)}
						atNs={atNs}
						selected={selectedHere}
						dimmed={dimmed}
						highlight={highlight}
						onSelect={onSelect}
					/>
				);
			})}
		</div>
	);
};
