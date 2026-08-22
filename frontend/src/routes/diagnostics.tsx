import { createFileRoute } from "@tanstack/react-router";
import { useEffect, useMemo, useRef, useState } from "react";
import type {
	ClockSnapshot,
	DiagnosticsFrame,
	ErrorSnapshot,
	HopSnapshot,
	QueueSnapshot,
} from "#/collections/types";
import {
	DiagnosticsGraph,
	type DiagnosticsSelection,
} from "#/components/dashboard/diagnostics-graph";
import { DiagnosticsWebRTCFeed } from "#/components/dashboard/diagnostics-transport";
import { Button } from "#/components/ui";

const DEFAULT_FRAME: DiagnosticsFrame = {
	status: "flowing",
	at_ns: 0,
	started_ns: 0,
	stages: [],
	hops: [],
	queues: [],
	errors: [],
	pass: { state: "idle" },
};

const STAGE_LABEL: Record<string, string> = {
	crypto: "Ingress",
	"websocket-api": "WebSocket API",
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
};

const QUEUE_KIND_TONE: Record<string, string> = {
	ingress: "bg-(--info)",
	rail: "bg-(--up)",
	derived: "bg-(--acc)",
	strategy: "bg-(--warn)",
	ui: "bg-(--brand)",
	broker: "bg-(--down)",
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

const formatCount = (count: number): string =>
	new Intl.NumberFormat("en", { notation: "compact" }).format(count);

const averageNanos = (clock?: {
	count?: number;
	total_ns?: number;
}): number => {
	if ((clock?.count ?? 0) <= 0) {
		return 0;
	}

	return (clock?.total_ns ?? 0) / (clock?.count ?? 1);
};

const formatAge = (atNs: number, lastAtNs: number | undefined): string => {
	if (lastAtNs === undefined || lastAtNs <= 0) {
		return "never";
	}

	const age = atNs - lastAtNs;

	if (age <= 0) {
		return "now";
	}

	if (age < 1_000_000_000) {
		return `${(age / 1_000_000).toFixed(0)}ms`;
	}

	return `${(age / 1_000_000_000).toFixed(1)}s`;
};

const errorsFor = (errors: ErrorSnapshot[], source: string): ErrorSnapshot[] =>
	errors.filter((error) => error.source === source);

const queueFill = (queue: QueueSnapshot): number => {
	const capacity = queue.cap;

	if (capacity !== undefined && capacity > 0) {
		return Math.min(100, (queue.depth / capacity) * 100);
	}

	if (queue.high_water <= 0) {
		return 0;
	}

	return Math.min(100, (queue.depth / queue.high_water) * 100);
};

const Metric = ({
	label,
	value,
	tone = "text-(--f1)",
}: {
	label: string;
	value: string;
	tone?: string;
}) => (
	<div className="border-(--line) border-b py-1.5 last:border-b-0">
		<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
			{label}
		</div>
		<div
			className={`mt-0.5 min-h-[1lh] font-mono text-[11px] tabular-nums ${tone}`}
		>
			{value}
		</div>
	</div>
);

const StageMini = ({
	name,
	stage,
	onSelect,
}: {
	name: string;
	stage: ClockSnapshot | undefined;
	onSelect: (selection: DiagnosticsSelection) => void;
}) => (
	<Button
		variant="bare"
		onClick={() => onSelect({ kind: "stage", name })}
		className="rounded-sm px-0.5 py-0.5 font-mono text-[8px] text-(--f2) hover:bg-(--raised)"
	>
		{STAGE_LABEL[name] ?? name}
		<span className="ml-1 inline-block w-[7ch] text-right tabular-nums text-(--f4)">
			{formatNanos(averageNanos(stage))}
		</span>
	</Button>
);

const QueueDetail = ({
	queue,
	delta,
	stages,
	onSelect,
}: {
	queue: QueueSnapshot;
	delta: number;
	stages: Map<string, ClockSnapshot>;
	onSelect: (selection: DiagnosticsSelection) => void;
}) => {
	const capacity = queue.cap;

	return (
		<>
			<div className="border-(--line) border-b px-3 py-2.5">
				<div className="flex items-center gap-2">
					<span
						className={`size-2 rounded-full ${QUEUE_KIND_TONE[queue.kind] ?? "bg-(--f4)"}`}
					/>
					<span className="truncate font-mono text-[12px] font-bold text-(--f1)">
						{queue.name}
					</span>
					<span className="ml-auto font-mono text-[8px] uppercase text-(--f4)">
						{queue.kind} queue
					</span>
				</div>
			</div>
			<div className="min-h-0 flex-1 overflow-auto px-3">
				<div className="grid grid-cols-2 gap-x-3">
					<Metric label="pending now" value={queue.depth.toLocaleString()} />
					<Metric
						label="per heartbeat"
						value={`${delta > 0 ? "+" : ""}${delta.toLocaleString()}`}
						tone={
							delta > 0
								? "text-(--warn)"
								: delta < 0
									? "text-(--up)"
									: "text-(--f3)"
						}
					/>
					<Metric
						label="session high-water"
						value={queue.high_water.toLocaleString()}
					/>
					<Metric
						label="capacity"
						value={capacity?.toLocaleString() ?? "unbounded"}
					/>
					<Metric
						label="fill"
						value={`${Math.round(queueFill(queue))}%`}
						tone={queueFill(queue) >= 80 ? "text-(--warn)" : "text-(--f2)"}
					/>
					<Metric
						label="current state"
						value={delta > 0 ? "growing" : delta < 0 ? "draining" : "unchanged"}
						tone={delta > 0 ? "text-(--warn)" : "text-(--f2)"}
					/>
					<Metric
						label="symbol queues"
						value={queue.symbols?.toLocaleString() ?? "—"}
					/>
					<Metric label="kind" value={queue.kind} />
				</div>
				<div className="border-(--line) border-b py-2">
					<div className="mb-1 font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						writers · own processing average
					</div>
					<div className="flex flex-wrap gap-1">
						{queue.writers.map((name) => (
							<StageMini
								key={name}
								name={name}
								stage={stages.get(name)}
								onSelect={onSelect}
							/>
						))}
					</div>
				</div>
				<div className="border-(--line) border-b py-2">
					<div className="mb-1 font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						readers · own processing average
					</div>
					<div className="flex flex-wrap gap-1">
						{queue.readers.map((name) => (
							<StageMini
								key={name}
								name={name}
								stage={stages.get(name)}
								onSelect={onSelect}
							/>
						))}
					</div>
				</div>
			</div>
		</>
	);
};

const HopReading = ({ hop }: { hop: HopSnapshot }) => (
	<div className="grid grid-cols-[minmax(0,1fr)_auto] gap-2 border-(--line) border-b py-1.5 last:border-b-0">
		<div className="min-w-0 font-mono text-[9px] text-(--f2)">
			<span className="text-(--f4)">{STAGE_LABEL[hop.from] ?? hop.from}</span>
			<span className="px-1 text-(--acc)">→</span>
			<span>{STAGE_LABEL[hop.to] ?? hop.to}</span>
		</div>
		<div className="font-mono text-[9px] tabular-nums text-(--f1)">
			{formatNanos(averageNanos(hop))} avg
		</div>
	</div>
);

const StageDetail = ({
	name,
	frame,
	stages,
	onSelect,
}: {
	name: string;
	frame: DiagnosticsFrame;
	stages: Map<string, ClockSnapshot>;
	onSelect: (selection: DiagnosticsSelection) => void;
}) => {
	const stage = stages.get(name);
	const errors = errorsFor(frame.errors ?? [], name);
	const inputs = (frame.queues ?? []).filter((queue) =>
		queue.readers.includes(name),
	);
	const outputs = (frame.queues ?? []).filter((queue) =>
		queue.writers.includes(name),
	);
	const writes = (frame.hops ?? []).filter(
		(hop) => hop.from === name && hop.count > 0,
	);
	const state =
		errors.length > 0
			? "error"
			: (stage?.active ?? 0) > 0
				? "running"
				: (stage?.count ?? 0) > 0
					? "observed"
					: "unseen";

	return (
		<>
			<div className="border-(--line) border-b px-3 py-2.5">
				<div className="flex items-center gap-2">
					<span
						className={`size-2 rounded-full ${
							state === "error"
								? "bg-(--down)"
								: state === "running"
									? "bg-(--info)"
									: state === "observed"
										? "bg-(--up)"
										: "bg-(--line2)"
						}`}
					/>
					<span className="font-mono text-[12px] font-bold text-(--f1)">
						{STAGE_LABEL[name] ?? name}
					</span>
					<span className="ml-auto font-mono text-[8px] uppercase text-(--f4)">
						{state}
					</span>
				</div>
			</div>
			<div className="min-h-0 flex-1 overflow-auto px-3">
				<div className="grid grid-cols-2 gap-x-3">
					<Metric
						label="processing average"
						value={formatNanos(averageNanos(stage))}
					/>
					<Metric label="processing last" value={formatNanos(stage?.last_ns)} />
					<Metric label="processing max" value={formatNanos(stage?.max_ns)} />
					<Metric
						label="operations"
						value={(stage?.count ?? 0).toLocaleString()}
					/>
					<Metric
						label="active operations"
						value={(stage?.active ?? 0).toLocaleString()}
					/>
					<Metric
						label="current elapsed"
						value={formatNanos(
							(stage?.active ?? 0) > 0 && (stage?.started_ns ?? 0) > 0
								? Math.max(0, (frame.at_ns ?? 0) - (stage?.started_ns ?? 0))
								: 0,
						)}
					/>
					<Metric
						label="last observed"
						value={formatAge(frame.at_ns ?? 0, stage?.last_at_ns)}
					/>
					<Metric
						label="reported errors"
						value={errors.length.toLocaleString()}
					/>
				</div>
				<div className="border-(--line) border-b py-2">
					<div className="mb-1 font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						input queues
					</div>
					<div className="flex flex-wrap gap-1">
						{inputs.map((queue) => (
							<Button
								key={queue.name}
								variant="outline"
								size="xxs"
								onClick={() => onSelect({ kind: "queue", name: queue.name })}
							>
								{queue.name} · {formatCount(queue.depth)}
							</Button>
						))}
						{inputs.length === 0 ? (
							<span className="font-mono text-[9px] text-(--f4)">
								none reported
							</span>
						) : null}
					</div>
				</div>
				<div className="border-(--line) border-b py-2">
					<div className="mb-1 font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						output queues
					</div>
					<div className="flex flex-wrap gap-1">
						{outputs.map((queue) => (
							<Button
								key={queue.name}
								variant="outline"
								size="xxs"
								onClick={() => onSelect({ kind: "queue", name: queue.name })}
							>
								{queue.name} · {formatCount(queue.depth)}
							</Button>
						))}
						{outputs.length === 0 ? (
							<span className="font-mono text-[9px] text-(--f4)">
								none reported
							</span>
						) : null}
					</div>
				</div>
				<div className="border-(--line) border-b py-2">
					<div className="mb-1 font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						write / handoff time
					</div>
					{writes.map((hop) => (
						<HopReading key={`${hop.from}-${hop.to}`} hop={hop} />
					))}
					{writes.length === 0 ? (
						<div className="font-mono text-[9px] leading-relaxed text-(--f4)">
							No write timing has been observed for this stage.
						</div>
					) : null}
				</div>
				{errors.map((error) => (
					<div
						key={`${error.at_ns}-${error.message}`}
						className="mb-2 border border-(--down)/40 bg-(--down)/10 p-2 font-mono text-[9px] text-(--down)"
					>
						{error.message}
					</div>
				))}
			</div>
		</>
	);
};

const OverviewDetail = ({ frame }: { frame: DiagnosticsFrame }) => {
	const latestError = frame.errors?.[0];
	const owners = frame.goroutines ?? [];
	const total = owners.reduce((sum, owner) => sum + owner.count, 0);

	return (
		<>
			<div className="border-(--line) border-b px-3 py-2.5 font-mono text-[12px] font-bold text-(--f1)">
				Inspection
			</div>
			<div className="px-3 py-3 font-mono text-[9px] leading-relaxed text-(--f3)">
				Select a stage to see who feeds it (amber) and who it feeds (blue).
				Select a queue to see its writers, readers, and live pressure.
				{latestError ? (
					<div className="mt-3 border border-(--down)/40 bg-(--down)/10 p-2 text-(--down)">
						<span className="font-bold uppercase">{latestError.source}</span>
						<br />
						{latestError.message}
					</div>
				) : null}
			</div>
			<div className="border-(--line) border-b px-3 py-2.5 font-mono text-[12px] font-bold text-(--f1)">
				Goroutines <span className="text-(--acc)">{total.toLocaleString()}</span>
			</div>
			<div className="min-h-0 flex-1 overflow-auto px-3 py-2">
				{owners.length === 0 ? (
					<div className="font-mono text-[9px] text-(--f4)">
						No goroutine inventory reported yet.
					</div>
				) : (
					<div className="flex flex-col gap-px">
						{owners.map((owner) => (
							<div
								key={owner.owner}
								className="grid grid-cols-[minmax(0,1fr)_auto] items-baseline gap-2 border-(--line)/40 border-b py-1 last:border-b-0"
								title={owner.state}
							>
								<span className="truncate text-(--f2)">{owner.owner}</span>
								<span className="text-right tabular-nums text-(--acc)">
									{owner.count.toLocaleString()}
								</span>
							</div>
						))}
					</div>
				)}
			</div>
		</>
	);
};

const DetailPanel = ({
	frame,
	selection,
	deltas,
	onSelect,
}: {
	frame: DiagnosticsFrame;
	selection: DiagnosticsSelection | null;
	deltas: Map<string, number>;
	onSelect: (selection: DiagnosticsSelection) => void;
}) => {
	const stages = useMemo(
		() => new Map((frame.stages ?? []).map((stage) => [stage.name, stage])),
		[frame.stages],
	);

	if (selection?.kind === "queue") {
		const queue = (frame.queues ?? []).find(
			(candidate) => candidate.name === selection.name,
		);

		if (queue) {
			return (
				<QueueDetail
					queue={queue}
					delta={deltas.get(queue.name) ?? 0}
					stages={stages}
					onSelect={onSelect}
				/>
			);
		}
	}

	if (selection?.kind === "stage") {
		return (
			<StageDetail
				name={selection.name}
				frame={frame}
				stages={stages}
				onSelect={onSelect}
			/>
		);
	}

	return <OverviewDetail frame={frame} />;
};

const Legend = () => (
	<div className="flex h-full flex-wrap items-center gap-x-3 gap-y-1 px-3 font-mono text-[8px] uppercase tracking-wide text-(--f4)">
		<span className="flex items-center gap-1">
			<span className="h-0.5 w-4 animate-pulse rounded bg-(--acc)" /> flowing
			(dashed)
		</span>
		<span className="flex items-center gap-1">
			<span className="h-0.5 w-4 rounded bg-(--f3)" /> idle (solid)
		</span>
		<span className="flex items-center gap-1">
			<span className="h-0.5 w-4 rounded bg-[hsl(140_32%_62%)]" /> healthy
			latency
		</span>
		<span className="flex items-center gap-1">
			<span className="h-0.5 w-4 rounded bg-[hsl(48_42%_61%)]" /> slight
			latency
		</span>
		<span className="flex items-center gap-1">
			<span className="h-0.5 w-4 rounded bg-[hsl(0_44%_64%)]" /> high latency
		</span>
		<span className="flex items-center gap-1">
			<span className="size-1.5 rounded-full bg-(--up)" /> live
		</span>
		<span className="flex items-center gap-1">
			<span className="size-1.5 rounded-full bg-(--info)" /> running
		</span>
		<span className="flex items-center gap-1">
			<span className="size-1.5 rounded-full bg-(--warn)" /> stale
		</span>
		<span className="flex items-center gap-1">
			<span className="size-1.5 rounded-full bg-(--down)" /> error
		</span>
	</div>
);

const StatusStrip = ({
	frame,
	connection,
	deltas,
	enabled,
	onToggle,
}: {
	frame: DiagnosticsFrame;
	connection: RTCPeerConnectionState | "connecting";
	deltas: Map<string, number>;
	enabled: boolean;
	onToggle: (enabled: boolean) => void;
}) => {
	const queues = frame.queues ?? [];
	const pending = queues.reduce((total, queue) => total + queue.depth, 0);
	const growing = queues.filter(
		(queue) => (deltas.get(queue.name) ?? 0) > 0,
	).length;
	const draining = queues.filter(
		(queue) => (deltas.get(queue.name) ?? 0) < 0,
	).length;
	const observed = (frame.stages ?? []).filter(
		(stage) => stage.count > 0 || (stage.active ?? 0) > 0,
	).length;
	const errored = (frame.errors ?? []).length;
	const goroutines = (frame.goroutines ?? []).reduce(
		(total, owner) => total + owner.count,
		0,
	);
	const lifetime =
		frame.started_ns && frame.at_ns
			? formatNanos(frame.at_ns - frame.started_ns)
			: "—";

	return (
		<div className="flex h-8 shrink-0 items-center gap-4 border-(--line) border-b px-3 font-mono text-[9px] uppercase tracking-wide text-(--f4)">
			<span
				className={connection === "connected" ? "text-(--up)" : "text-(--warn)"}
			>
				● {connection === "connected" ? "wired" : connection}
			</span>
			<span>
				status{" "}
				<strong className="text-(--f1)">{frame.status ?? "flowing"}</strong>
			</span>
			<span>
				live{" "}
				<strong className="inline-block w-[2ch] text-right tabular-nums text-(--up)">
					{observed}
				</strong>{" "}
				/ {frame.stages?.length ?? 0}
			</span>
			<span>
				pending{" "}
				<strong className="inline-block w-[12ch] text-right tabular-nums text-(--info)">
					{pending.toLocaleString()}
				</strong>
			</span>
			<span>
				growing{" "}
				<strong
					className={`inline-block w-[2ch] text-right tabular-nums ${growing > 0 ? "text-(--warn)" : "text-(--f1)"}`}
				>
					{growing}
				</strong>
			</span>
			<span>
				draining{" "}
				<strong
					className={`inline-block w-[2ch] text-right tabular-nums ${draining > 0 ? "text-(--up)" : "text-(--f1)"}`}
				>
					{draining}
				</strong>
			</span>
			<span>
				goroutines{" "}
				<strong className="inline-block w-[5ch] text-right tabular-nums text-(--acc)">
					{formatCount(goroutines)}
				</strong>
			</span>
			{errored > 0 ? (
				<span>
					errors{" "}
					<strong className="inline-block w-[2ch] text-right tabular-nums text-(--down)">
						{errored}
					</strong>
				</span>
			) : null}
			<span>
				pass{" "}
				<strong className="text-(--f1)">{frame.pass?.state ?? "idle"}</strong>
			</span>
			<button
				type="button"
				onClick={() => onToggle(!enabled)}
				className={`ml-auto flex cursor-pointer items-center gap-1.5 rounded-sm border px-1.5 py-0.5 uppercase ${enabled ? "border-(--up)/60 text-(--up)" : "border-(--warn)/60 text-(--warn)"}`}
				title={
					enabled
						? "Switch diagnostics collection off"
						: "Switch diagnostics collection on"
				}
			>
				<span
					className={`size-1.5 rounded-full ${enabled ? "bg-(--up)" : "bg-(--warn)"}`}
				/>
				{enabled ? "on" : "off"}
			</button>
			<span>
				uptime{" "}
				<span className="inline-block w-[9ch] text-right tabular-nums">
					{lifetime}
				</span>
			</span>
		</div>
	);
};

/*
DiagnosticsControlURL locates the hub's runtime on/off endpoint, mirroring the
WebRTC signaling origin (env override with a localhost default).
*/
const diagnosticsControlURL = () =>
	import.meta.env.VITE_SYMM_WEBRTC_URL?.trim().replace(/\/webrtc\/manifold$/, "") ||
	"http://127.0.0.1:8765";

/*
DiagnosticsDataflow renders the live analytical data plane as a wiring graph:
signals, logic, strategy, desks, and the queues between them, with edges that
animate with the direction and state of flow. The server-reported queue
topology is the only source of edges; nothing is invented.
*/
export const DiagnosticsDataflow = ({
	frame,
	connection = "connected",
	enabled = true,
	onToggleEnabled,
}: {
	frame: DiagnosticsFrame;
	connection?: RTCPeerConnectionState | "connecting";
	enabled?: boolean;
	onToggleEnabled?: (enabled: boolean) => void;
}) => {
	const [selection, setSelection] = useState<DiagnosticsSelection | null>(null);
	const previousDepths = useRef(new Map<string, number>());
	const previousHops = useRef(new Map<string, number>());

	const queueDeltas = useMemo(() => {
		return new Map(
			(frame.queues ?? []).map((queue) => [
				queue.name,
				previousDepths.current.has(queue.name)
					? queue.depth - (previousDepths.current.get(queue.name) ?? 0)
					: 0,
			]),
		);
	}, [frame.queues]);

	const hopDeltas = useMemo(() => {
		return new Map(
			(frame.hops ?? []).map((hop) => {
				const key = `${hop.from}>${hop.to}`;
				return [
					key,
					previousHops.current.has(key)
						? hop.count - (previousHops.current.get(key) ?? 0)
						: 0,
				];
			}),
		);
	}, [frame.hops]);

	useEffect(() => {
		previousDepths.current = new Map(
			(frame.queues ?? []).map((queue) => [queue.name, queue.depth]),
		);
		previousHops.current = new Map(
			(frame.hops ?? []).map((hop) => [`${hop.from}>${hop.to}`, hop.count]),
		);
	}, [frame.queues, frame.hops]);

	return (
		<div className="grid h-full max-h-[calc(100dvh-3.25rem)] min-h-0 min-w-230 grid-cols-[minmax(0,1fr)_minmax(280px,22vw)] overflow-hidden bg-(--bg)">
			<section className="flex min-h-0 min-w-0 flex-col overflow-hidden border-(--line) border-r">
				<StatusStrip
					frame={frame}
					connection={connection}
					deltas={queueDeltas}
					enabled={enabled}
					onToggle={onToggleEnabled ?? (() => {})}
				/>
				<div className="flex h-6 shrink-0 items-center border-(--line) border-b bg-(--surface)">
					<Legend />
				</div>
				<div className="min-h-0 flex-1 overflow-hidden p-6">
					{enabled ? (
						<DiagnosticsGraph
							frame={frame}
							queueDeltas={queueDeltas}
							hopDeltas={hopDeltas}
							selection={selection}
							onSelect={setSelection}
						/>
					) : (
						<div className="flex h-full items-center justify-center">
							<div className="text-center font-mono">
								<div className="text-[10px] uppercase tracking-widest text-(--f4)">
									Diagnostics collection is switched off
								</div>
								<Button
									variant="outline"
									onClick={() => onToggleEnabled?.(true)}
									className="mt-3"
								>
									Switch on
								</Button>
							</div>
						</div>
					)}
				</div>
			</section>
			<aside className="flex min-h-0 flex-col overflow-hidden bg-(--surface)">
				<DetailPanel
					frame={frame}
					selection={selection}
					deltas={queueDeltas}
					onSelect={setSelection}
				/>
			</aside>
		</div>
	);
};

const DiagnosticsSurface = () => {
	const [frame, setFrame] = useState<DiagnosticsFrame>(DEFAULT_FRAME);
	const [connection, setConnection] = useState<
		RTCPeerConnectionState | "connecting"
	>("connecting");
	const [enabled, setEnabled] = useState<boolean>(true);

	useEffect(() => {
		const feed = new DiagnosticsWebRTCFeed({
			onFrame: (next) => {
				setFrame(next);

				if (next.enabled !== undefined) {
					setEnabled(next.enabled);
				}
			},
			onState: setConnection,
			onError: (error) => {
				setConnection("connecting");
				console.warn("diagnostics WebRTC:", error.message);
			},
		});
		feed.connect();

		return () => feed.close();
	}, []);

	useEffect(() => {
		const controller = new AbortController();

		fetch(`${diagnosticsControlURL()}/diagnostics`, {
			signal: controller.signal,
		})
			.then((response) => response.ok && response.json())
			.then((state: { enabled?: boolean } | false) => {
				if (state && typeof state.enabled === "boolean") {
					setEnabled(state.enabled);
				}
			})
			.catch((error: unknown) => {
				if (
					error instanceof DOMException &&
					error.name === "AbortError"
				) {
					return;
				}

				console.warn(
					"diagnostics state read:",
					error instanceof Error ? error.message : String(error),
				);
			});

		return () => controller.abort();
	}, []);

	const toggleEnabled = async (next: boolean) => {
		setEnabled(next);

		try {
			const response = await fetch(`${diagnosticsControlURL()}/diagnostics`, {
				method: "POST",
				headers: { "Content-Type": "application/json" },
				body: JSON.stringify({ enabled: next }),
			});

			if (!response.ok) {
				throw new Error(`diagnostics toggle failed with ${response.status}`);
			}
		} catch (error) {
			console.warn(
				"diagnostics toggle:",
				error instanceof Error ? error.message : String(error),
			);
		}
	};

	return (
		<DiagnosticsDataflow
			frame={frame}
			connection={connection}
			enabled={enabled}
			onToggleEnabled={toggleEnabled}
		/>
	);
};

export const Route = createFileRoute("/diagnostics")({
	component: DiagnosticsSurface,
});
