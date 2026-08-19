import { createFileRoute } from "@tanstack/react-router";
import {
	type ReactNode,
	useEffect,
	useMemo,
	useRef,
	useState,
} from "react";
import type {
	ClockSnapshot,
	DiagnosticsFrame,
	ErrorSnapshot,
	HopSnapshot,
	PassStatus,
} from "#/collections/types";
import { DiagnosticsWebRTCFeed } from "#/components/dashboard/diagnostics-transport";

const DEFAULT_FRAME: DiagnosticsFrame = {
	status: "flowing",
	at_ns: 0,
	started_ns: 0,
	stages: [],
	hops: [],
	errors: [],
	pass: { state: "idle" },
};

/*
displayName is the friendly label shown on each node. Signals carry their own
wire source names.
*/
const MODULE_LABEL: Record<string, string> = {
	crypto: "Ingress",
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
	planner: "Planner",
	mcts: "MCTS",
	allocation: "Allocation",
	desk: "Desk",
	measurements: "Measurements",
};

/*
LANES groups nodes into horizontal pipeline columns. The signals rail is one
fan-in/fan-out stage; the logic/strategy/execution stages follow.
*/
const LANES: Array<{
	label: string;
	hint: string;
	nodes: string[];
	intra?: string[];
}> = [
	{ label: "ingress", hint: "market data", nodes: ["crypto"] },
	{
		label: "signals",
		hint: "conditioner rail",
		nodes: [
			"correlation",
			"cvd",
			"depthflow",
			"exhaustion",
			"hawkes",
			"leadlag",
			"liquidity",
			"pumpdump",
			"sentiment",
			"toxicity",
		],
		intra: [
			"correlation",
			"cvd",
			"depthflow",
			"exhaustion",
			"hawkes",
			"leadlag",
			"liquidity",
			"pumpdump",
			"sentiment",
			"toxicity",
		],
	},
	{ label: "logic · G1", hint: "dimensionality + manifold", nodes: ["category", "manifold"] },
	{ label: "logic · G2", hint: "causal + cognition", nodes: ["causal", "cognition"] },
	{ label: "logic · G3", hint: "graph assembly", nodes: ["graph"] },
	{ label: "strategy", hint: "search · size · send", nodes: ["planner", "mcts", "allocation"] },
	{ label: "execution", hint: "broker desk", nodes: ["desk"] },
];

/*
EDGES are the wired hop intervals the server measures. It is the honest latency
signal between systems (as opposed to the work time inside a node).
*/
const EDGES: Array<{ from: string; to: string }> = [
	{ from: "crypto", to: "measurements" },
	{ from: "measurements", to: "category" },
	{ from: "category", to: "causal" },
	{ from: "causal", to: "graph" },
	{ from: "graph", to: "planner" },
	{ from: "planner", to: "mcts" },
	{ from: "mcts", to: "allocation" },
	{ from: "allocation", to: "desk" },
];

const NODE_W = 150;
const NODE_H = 74;
const NODE_GAP = 18;
const COLUMN_GAP = 210;
const RAIL_TOP_PAD = 40;

type Point = { x: number; y: number };
type Layout = { x: number; y: number };

const describe = (): Record<string, Layout> => {
	const positions: Record<string, Layout> = {};
	const heights = LANES.map((lane) => {
		const count = lane.nodes.length;

		return count > 0 ? count * NODE_H + (count - 1) * NODE_GAP : 0;
	});
	const maxHeight = Math.max(...heights, 1);
	const yCenter = RAIL_TOP_PAD + maxHeight / 2;

	LANES.forEach((lane, column) => {
		const count = lane.nodes.length;

		if (count === 0) {
			return;
		}

		const total = count * NODE_H + (count - 1) * NODE_GAP;
		const top = yCenter - total / 2;
		const x = (column * COLUMN_GAP) + NODE_W / 2;

		lane.nodes.forEach((name, index) => {
			positions[name] = {
				x,
				y: top + index * (NODE_H + NODE_GAP),
			};
		});
	});

	// The measurements stage is the signals rail's bounding box.
	if (positions["correlation"]) {
		positions["measurements"] = {
			x: (1 * COLUMN_GAP) + NODE_W / 2,
			y: yCenter,
		};
	}

	return positions;
};

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

const averageNanos = (clock: {
	count?: number;
	total_ns?: number;
}): number => {
	if ((clock.count ?? 0) <= 0) {
		return 0;
	}

	return (clock.total_ns ?? 0) / (clock.count ?? 1);
};

const stageMap = (frame: DiagnosticsFrame): Map<string, ClockSnapshot> =>
	new Map((frame.stages ?? []).map((stage) => [stage.name, stage]));

const hopBetween = (
	hops: HopSnapshot[],
	from: string,
	to: string,
): HopSnapshot | undefined => hops.find((hop) => hop.from === from && hop.to === to);

const errorCount = (
	errors: ErrorSnapshot[] = [],
	source: string,
): number => errors.filter((err) => err.source === source).length;

const formatAge = (atNs: number, lastAtNs: number | undefined): string => {
	if (lastAtNs === undefined || lastAtNs <= 0) {
		return "never";
	}

	const age = atNs - lastAtNs;

	if (age <= 0) {
		return "now";
	}

	return `${(age / 1_000_000).toFixed(0)}ms`;
};

/*
nodeActivity returns the traffic-light state for a node:
  error  → red (its section reported an error)
  stale  → amber (no observed work for several heartbeats — the idle state is
           "flowing / moving", so a long gap means the subsystem is stuck or idle)
  live   → green (actively producing per the heartbeat cadence)
*/
type Activity = "error" | "stale" | "live";

const activityOf = (
	stage: ClockSnapshot | undefined,
	atNs: number,
	errors: ErrorSnapshot[],
	name: string,
): Activity => {
	if (errorCount(errors, name) > 0) {
		return "error";
	}

	if (!stage || (stage.last_at_ns ?? 0) <= 0) {
		return "stale";
	}

	const age = atNs - (stage.last_at_ns ?? 0);

	if (age > 3_000_000_000) {
		return "stale";
	}

	return "live";
};

const ACTIVITY_TONE: Record<Activity, { dot: string; ring: string; label: string }> = {
	live: { dot: "bg-(--up)", ring: "shadow-[0_0_12px_rgba(34,197,94,0.45)]", label: "live" },
	stale: { dot: "bg-(--warn)", ring: "", label: "stale" },
	error: { dot: "bg-(--down)", ring: "shadow-[0_0_14px_rgba(239,68,68,0.55)]", label: "error" },
};

const NodeCard = ({
	name,
	stage,
	activity,
	atNs,
	errors,
}: {
	name: string;
	stage: ClockSnapshot | undefined;
	activity: Activity;
	atNs: number;
	errors: ErrorSnapshot[];
}) => {
	const tone = ACTIVITY_TONE[activity];
	const errorBadges = errorCount(errors, name);
	const mean = averageNanos(stage ?? { name, count: 0, total_ns: 0, last_ns: 0 });

	return (
		<div
			style={{ height: NODE_H - 4, width: NODE_W - 4 }}
			className={`relative flex flex-col justify-between rounded-md border px-2.5 py-2 ${
				activity === "error"
					? "border-(--down)/60 bg-(--down)/10"
					: activity === "stale"
						? "border-(--warn)/50 bg-(--surface)"
						: "border-(--line2) bg-(--surface)"
			} ${tone.ring}`}
		>
			<FlexRow className="items-center gap-1.5">
				<span className={`h-2 w-2 shrink-0 rounded-full ${tone.dot}`} />
				<span className="truncate font-mono text-[10px] font-semibold uppercase tracking-wider text-(--f1)">
					{MODULE_LABEL[name] ?? name}
				</span>
				{errorBadges > 0 ? (
					<span className="ml-auto flex h-3.5 min-w-3.5 items-center justify-center rounded-full bg-(--down) px-1 font-mono text-[8px] font-bold text-white">
						{errorBadges}
					</span>
				) : null}
			</FlexRow>
			<FlexRow justify="between" className="items-baseline">
				<span className="font-mono text-[13px] font-semibold tabular-nums text-(--acc)">
					{formatNanos(mean)}
				</span>
				<span className="font-mono text-[9px] uppercase text-(--f4)">
					avg
				</span>
			</FlexRow>
			<FlexRow justify="between" className="font-mono text-[8.5px] text-(--f4)">
				<span>now {formatNanos(stage?.last_ns)}</span>
				<span>age {formatAge(atNs, stage?.last_at_ns)}</span>
			</FlexRow>
		</div>
	);
};

const FlexRow = ({
	children,
	className = "",
	justify,
}: {
	children: ReactNode;
	className?: string;
	justify?: "between";
}) => (
	<div
		className={`flex ${justify === "between" ? "justify-between" : ""} ${className}`}
	>
		{children}
	</div>
);

const EdgePath = ({
	from,
	to,
	hop,
}: {
	from: Point;
	to: Point;
	hop: HopSnapshot | undefined;
}) => {
	const mean = hop ? averageNanos(hop) : 0;
	const last = hop?.last_ns ?? 0;
	const highLatency = mean > 150_000_000 && (hop?.count ?? 0) > 0;
	const startX = from.x + NODE_W / 2;
	const startY = from.y + NODE_H / 2;
	const endX = to.x - NODE_W / 2;
	const endY = to.y + NODE_H / 2;
	const midX = (startX + endX) / 2;
	const curve = `M ${startX} ${startY} C ${midX} ${startY}, ${midX} ${endY}, ${endX} ${endY}`;
	const color = highLatency ? "var(--warn)" : "var(--line2)";
	const dotted = (hop?.count ?? 0) === 0;

	return (
		<g>
			<path
				d={curve}
				fill="none"
				stroke={color}
				strokeWidth={highLatency ? 2 : 1.5}
				strokeDasharray={dotted ? "4 5" : undefined}
				markerEnd="url(#diag-arrow)"
			/>
			{(hop?.count ?? 0) > 0 ? (
				<g>
					<text
						x={midX}
						y={Math.min(startY, endY) - 6}
						textAnchor="middle"
						className="fill-(--f4) font-mono"
						fontSize={9}
					>
						wire {formatNanos(mean)}
					</text>
					<text
						x={midX}
						y={Math.min(startY, endY) + 6}
						textAnchor="middle"
						className="fill-(--f4) font-mono"
						fontSize={8}
					>
						last {formatNanos(last)}{last > 150_000_000 ? " · slow" : ""}
					</text>
				</g>
			) : null}
		</g>
	);
};

const WiringDiagram = ({
	frame,
	fromNode,
}: {
	frame: DiagnosticsFrame;
	fromNode: (name: string) => Layout;
}) => {
	const stages = stageMap(frame);
	const width = LANES.length * COLUMN_GAP;
	const maxHeight =
		Math.max(
			...LANES.map((lane) =>
				lane.nodes.length > 0
					? lane.nodes.length * NODE_H + (lane.nodes.length - 1) * NODE_GAP
					: 0,
			),
			1,
		) +
		RAIL_TOP_PAD * 2;

	return (
		<div className="overflow-x-auto">
			<svg
				viewBox={`0 0 ${width} ${maxHeight}`}
				className="min-w-[820px]"
				style={{ height: `${maxHeight}px` }}
			>
				<defs>
					<marker
						id="diag-arrow"
						viewBox="0 0 10 10"
						refX="9"
						refY="5"
						markerWidth="7"
						markerHeight="7"
						orient="auto-start-reverse"
					>
						<path d="M 0 0 L 10 5 L 0 10 z" fill="var(--line2)" />
					</marker>
				</defs>

				{EDGES.map((edge) => {
					const from = fromNode(edge.from);
					const to = fromNode(edge.to);

					if (!from || !to) {
						return null;
					}

					return (
						<EdgePath
							key={`${edge.from}-${edge.to}`}
							from={{ x: from.x, y: from.y }}
							to={{ x: to.x, y: to.y }}
							hop={hopBetween(frame.hops ?? [], edge.from, edge.to)}
						/>
					);
				})}

				{LANES.map((lane, column) => (
					<g key={column}>
						<text
							x={column * COLUMN_GAP + NODE_W / 2}
							y={10}
							textAnchor="middle"
							className="fill-(--f3) font-mono"
							fontSize={10}
						>
							{lane.label}
						</text>
						<text
							x={column * COLUMN_GAP + NODE_W / 2}
							y={24}
							textAnchor="middle"
							className="fill-(--f4) font-mono"
							fontSize={8}
						>
							{lane.hint}
						</text>
						{lane.nodes.map((name) => {
							const position = fromNode(name);

							if (!position) {
								return null;
							}

							return (
								<foreignObject
									key={name}
									x={position.x - NODE_W / 2}
									y={position.y}
									width={NODE_W}
									height={NODE_H}
								>
									<NodeCard
										name={name}
										stage={stages.get(name)}
										activity={activityOf(
											stages.get(name),
											frame.at_ns ?? 0,
											frame.errors ?? [],
											name,
										)}
										atNs={frame.at_ns ?? 0}
										errors={frame.errors ?? []}
									/>
								</foreignObject>
							);
						})}
					</g>
				))}
			</svg>
		</div>
	);
};

const Warning = ({
	errors,
	frame,
}: {
	errors: ErrorSnapshot[];
	frame: DiagnosticsFrame;
}) => {
	const latest = errors[0];

	if (!latest) {
		return null;
	}

	const when = latest.at_ns ? formatAge(frame.at_ns ?? 0, latest.at_ns) : "";

	return (
		<div className="flex items-center gap-3 rounded-md border border-(--down)/40 bg-(--down)/10 px-4 py-2.5">
			<span className="h-2.5 w-2.5 shrink-0 rounded-full bg-(--down)" />
			<div className="min-w-0">
				<div className="font-mono text-[11px] font-semibold uppercase tracking-wider text-(--down)">
					idiot-proofing · {latest.source} error
				</div>
				<div className="truncate font-mono text-[11px] text-(--f2)">
					{latest.message} {when !== "now" ? `(${when} ago)` : ""}
				</div>
			</div>
		</div>
	);
};

const PASS_TONE: Record<
	PassStatus["state"],
	{ dot: string; text: string; bar: string }
> = {
	idle: {
		dot: "bg-(--warn)",
		text: "text-(--warn)",
		bar: "border-(--warn)/40 bg-(--warn)/10",
	},
	running: {
		dot: "bg-(--up)",
		text: "text-(--up)",
		bar: "border-(--line2) bg-(--surface)",
	},
	blocked: {
		dot: "bg-(--down)",
		text: "text-(--down)",
		bar: "border-(--down)/50 bg-(--down)/10",
	},
};

/*
PassBanner surfaces the measurement engine's pass state — the single most
important discriminator between "nothing to do" (gated idle) and "stuck doing
nothing" (a pass started but never completed). A blocked pass is the liar that
hides behind every upstream stage sitting stale while planner/desk keep going.
*/
const PassBanner = ({ pass }: { pass?: PassStatus }) => {
	const state = pass?.state ?? "idle";
	const tone = PASS_TONE[state];

	let detail: string;

	if (state === "blocked") {
		detail = `a pass has been in flight for ${formatNanos(pass?.in_flight_ns)} — a signal or analyzer is stuck, not just quiet`;
	} else if (state === "running") {
		detail = `pass in flight ${formatNanos(pass?.in_flight_ns)} · last pass ${formatNanos(pass?.last_pass_ns)}`;
	} else {
		const sinceLast = pass?.since_last_ns ?? 0;
		detail = `no pending market rows · last pass ${formatNanos(pass?.last_pass_ns)} · idle ${sinceLast > 0 ? `${(sinceLast / 1_000_000_000).toFixed(1)}s` : "—"}`;
	}

	const label =
		state === "blocked"
			? "idiot-proofing · measurement pass blocked"
			: state === "running"
				? "measurement pass running"
				: "measurement pass idle";

	return (
		<div className={`flex items-center gap-3 rounded-md border px-4 py-2.5 ${tone.bar}`}>
			<span className={`h-2.5 w-2.5 shrink-0 rounded-full ${tone.dot}`} />
			<div className="min-w-0">
				<div className={`font-mono text-[11px] font-semibold uppercase tracking-wider ${tone.text}`}>
					{label}
				</div>
				<div className="truncate font-mono text-[11px] text-(--f2)">
					{detail}
				</div>
			</div>
		</div>
	);
};

export const DiagnosticsSurface = () => {
	const [frame, setFrame] = useState<DiagnosticsFrame>(DEFAULT_FRAME);
	const [state, setState] = useState<RTCPeerConnectionState | "connecting">(
		"connecting",
	);
	const feedRef = useRef<DiagnosticsWebRTCFeed | null>(null);

	useEffect(() => {
		const feed = new DiagnosticsWebRTCFeed({
			onFrame: setFrame,
			onState: setState,
			onError: (error) => {
				setState("connecting");
				// eslint-disable-next-line no-console
				console.warn("diagnostics WebRTC:", error.message);
			},
		});
		feedRef.current = feed;
		feed.connect();

		return () => feed.close();
	}, []);

	const layout = useMemo(() => describe(), []);
	const fromNode = (name: string): Layout => layout[name];
	const atNs = frame.at_ns ?? 0;
	const lifetime =
		frame.started_ns && atNs > 0 ? atNs - frame.started_ns : 0;
	const liveCount =
		frame.stages?.filter(
			(stage) => activityOf(stage, atNs, frame.errors ?? [], stage.name) === "live",
		).length ?? 0;

	return (
		<div className="flex h-full min-w-275 flex-col gap-4 overflow-auto bg-(--bg) p-5">
			<header className="flex items-center justify-between gap-4">
				<div className="flex flex-col gap-0.5">
					<span className="font-mono text-[10px] uppercase tracking-widest text-(--f4)">
						Analytical data plane · WebRTC
					</span>
					<h1 className="text-xl font-bold tracking-tight text-(--f1)">
						System diagnostics
					</h1>
				</div>
				<div className="flex items-center gap-3">
					<span className="font-mono text-[10px] text-(--f4)">
						{liveCount}/{frame.stages?.length ?? 0} live
					</span>
					<span
						className={`rounded-full px-2.5 py-1 font-mono text-[10px] font-semibold uppercase tracking-widest ${
							state === "connected"
								? "bg-(--up)/15 text-(--up)"
								: "bg-(--warn)/15 text-(--warn)"
						}`}
					>
						{state === "connected" ? "wired" : state}
					</span>
				</div>
			</header>

			<Warning errors={frame.errors ?? []} frame={frame} />

			<PassBanner pass={frame.pass} />

			<div className="flex flex-wrap items-center gap-4 font-mono text-[9px] uppercase tracking-wider text-(--f4)">
				<span className="flex items-center gap-1.5">
					<span className="h-2 w-2 rounded-full bg-(--up)" /> live
				</span>
				<span className="flex items-center gap-1.5">
					<span className="h-2 w-2 rounded-full bg-(--warn)" /> idle/stale
				</span>
				<span className="flex items-center gap-1.5">
					<span className="h-2 w-2 rounded-full bg-(--down)" /> error
				</span>
				<span className="flex items-center gap-1.5">
					<span className="h-px w-6 bg-(--line2)" /> wire latency
				</span>
				<span className="flex items-center gap-1.5">
					<span className="h-px w-6 border-t border-dashed border-(--line2)" />
					unmeasured hop
				</span>
			</div>

			<div className="rounded-md border border-(--line) bg-(--surface) p-4">
				<div className="mb-3 flex items-baseline justify-between">
					<div>
						<div className="font-mono text-[10px] font-semibold uppercase tracking-wider text-(--f3)">
							Wiring map
						</div>
						<div className="font-mono text-[9px] uppercase text-(--f4)">
							node = mean/iteration · now/age = current wall-clock · edge = wire latency
						</div>
					</div>
					<span className="font-mono text-[9px] uppercase text-(--f4)">
						up {lifetime > 0 ? `${(lifetime / 1_000_000_000).toFixed(0)}s` : "—"}
					</span>
				</div>
				<WiringDiagram frame={frame} fromNode={fromNode} />
			</div>
		</div>
	);
};

export const Route = createFileRoute("/diagnostics")({
	component: DiagnosticsSurface,
});
