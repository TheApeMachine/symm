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

const HALF = { w: 6.5, h: 4.5 };

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

	const layerCount = Math.max(1, byLayer.size);
	const placements = new Map<string, Placement>();

	for (const [l, ids] of byLayer) {
		// Stable, readable order within a layer: most-connected first, then
		// alphabetical — keeps a hub node centered rather than jittering
		// position as unrelated nodes come and go.
		const ordered = [...ids].sort((a, b) => {
			const degreeA = (outgoing.get(a)?.length ?? 0) + (incoming.get(a)?.length ?? 0);
			const degreeB = (outgoing.get(b)?.length ?? 0) + (incoming.get(b)?.length ?? 0);
			return degreeB - degreeA || a.localeCompare(b);
		});

		const y = ((l + 1) / (layerCount + 1)) * 100;

		ordered.forEach((id, index) => {
			const x = ((index + 1) / (ordered.length + 1)) * 100;
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

const stubPoint = (point: Point, side: NodeSide): Point => {
	if (side === "top") return { x: point.x, y: point.y - 1.0 };
	if (side === "bottom") return { x: point.x, y: point.y + 1.0 };
	if (side === "left") return { x: point.x - 1.0, y: point.y };
	return { x: point.x + 1.0, y: point.y };
};

const pathOf = (points: Point[]): string =>
	points
		.map((point, index) => `${index === 0 ? "M" : "L"} ${point.x.toFixed(3)} ${point.y.toFixed(3)}`)
		.join(" ");

const labelPointOf = (points: Point[]): Point => {
	const mid = Math.floor(points.length / 2);
	const a = points[Math.max(0, mid - 1)];
	const b = points[mid];
	return { x: (a.x + b.x) / 2, y: (a.y + b.y) / 2 };
};

/*
routeEdge draws a clean 2-bend orthogonal route between two ports: straight
down/across from the source, a single elbow, straight into the target. With
positions auto-laid-out into clean layers (see autoLayout), a simple elbow
route stays legible without the old fixed-topology router's obstacle-avoidance
machinery — that complexity existed to route around a large, hand-placed,
overlapping diagram; a layered auto-layout keeps lanes naturally separated.
*/
const routeEdge = (fromPort: Point, toPort: Point, fromSide: NodeSide, toSide: NodeSide): Point[] => {
	const start = stubPoint(fromPort, fromSide);
	const end = stubPoint(toPort, toSide);

	if (Math.abs(start.x - end.x) < 0.001 || Math.abs(start.y - end.y) < 0.001) {
		return [fromPort, start, end, toPort];
	}

	const vertical = fromSide === "top" || fromSide === "bottom";
	const bend = vertical ? { x: start.x, y: end.y } : { x: end.x, y: start.y };

	return [fromPort, start, bend, end, toPort];
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

	for (const edge of edgeList) {
		const id = `${edge.from}>${edge.to}`;
		const from = placements.get(edge.from);
		const to = placements.get(edge.to);
		const side = sides.get(id);
		const fromPort = portsMap.get(`${id}:from`);
		const toPort = portsMap.get(`${id}:to`);
		if (!from || !to || !side || !fromPort || !toPort) continue;

		const points = routeEdge(fromPort, toPort, side.from, side.to);

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

	return { placements, edges: edgesOut, ports };
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

const StageNode = ({
	placement,
	stage,
	state,
	selected,
	dimmed,
	highlight,
	onSelect,
}: {
	placement: Placement;
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
				selected && "outline outline-(--acc) outline-offset-1 ring-1 ring-(--acc)/40",
				highlight === "up" && !selected && "outline outline-(--warn)/70 outline-offset-1",
				highlight === "down" && !selected && "outline outline-(--info)/70 outline-offset-1",
				dimmed && "opacity-20",
			)}
			style={{
				left: `${placement.x}%`,
				top: `${placement.y}%`,
				width: `${HALF.w * 2}%`,
				height: `${HALF.h * 2}%`,
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
				<span className="truncate font-mono text-[9px] font-semibold uppercase tracking-wide text-(--f1)">
					{placement.label}
				</span>
			</div>
			<div className="relative grid grid-cols-[3ch_7ch_6ch] items-baseline gap-1 font-mono">
				<span className="text-[7px] uppercase text-(--f4)">rate</span>
				<span
					className={cn(
						"text-right text-[10px] font-bold tabular-nums text-(--acc)",
						stage === undefined && "text-(--f4)",
					)}
				>
					{stage === undefined ? "—" : formatRate(stage.avgGapNs)}
				</span>
				<span className={`truncate text-right text-[7px] uppercase ${tone.text}`}>{state}</span>
			</div>
			<div className="relative grid grid-cols-[3ch_7ch_6ch] items-baseline gap-1 font-mono text-[7px] text-(--f4)">
				<span>last</span>
				<span className="text-right tabular-nums">{formatNanos(stage?.lastGapNs)}</span>
				<span className="truncate text-right tabular-nums">
					{stage !== undefined ? `${formatCount(stage.seqCount)} ops` : "unseen"}
				</span>
			</div>
			<div className="relative grid grid-cols-[3ch_7ch_6ch] items-baseline gap-1 font-mono text-[7px] text-(--f4)">
				<span>bklg</span>
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
				<span className="truncate text-right tabular-nums">peak {formatCount(maxBacklog)}</span>
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
				strokeOpacity={dimmed && !hovered ? 0.15 : 0.65}
				vectorEffect="non-scaling-stroke"
				pathLength={100}
				strokeLinecap="round"
				strokeLinejoin="round"
				strokeDasharray={flowing ? "4 5" : undefined}
				className={cn("diag-edge transition-all", flowing && "diag-flow", !flowing && "diag-solid")}
			/>
		</g>
	);
};

const EdgeLatency = ({ edge, dimmed, hovered }: { edge: DiagEdge; dimmed: boolean; hovered: boolean }) => {
	if (edge.stats.avgLatencyNs <= 0) return null;

	return (
		<div
			className={cn(
				"pointer-events-none absolute z-5 min-w-[7ch] -translate-x-1/2 -translate-y-1/2 rounded-sm border border-(--line) bg-(--bg)/95 px-1 py-px text-center font-mono text-[7px] tabular-nums text-(--f3) shadow-sm transition-all",
				dimmed && !hovered && "opacity-15",
				hovered && "border-(--acc) text-(--acc) opacity-100 z-20 scale-110",
			)}
			style={{ left: `${edge.labelPoint.x}%`, top: `${edge.labelPoint.y}%` }}
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
		<div className="relative h-full w-full overflow-hidden select-none" onClick={() => onSelect(null)}>
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
			<svg viewBox="0 0 100 100" preserveAspectRatio="none" className="absolute inset-0 h-full w-full" aria-hidden="true">
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
					<EdgeLatency key={`latency:${edge.id}`} edge={edge} dimmed={selection !== null && !connected} hovered={hovered} />
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
	);
};
