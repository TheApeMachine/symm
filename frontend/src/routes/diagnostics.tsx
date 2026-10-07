import { createFileRoute } from "@tanstack/react-router";
import { useSelector, useStore } from "@tanstack/react-store";
import { useState } from "react";
import { onlineAtom } from "#/collections/app";
import type { EdgeStats, NodeStats } from "#/collections/topology";
import { topologyStore } from "#/collections/topology";
import {
	backlogTone,
	concurrentSiblings,
	DiagnosticsGraph,
	type DiagnosticsSelection,
	formatCount,
	formatNanos,
	formatRate,
	isHop,
} from "#/components/dashboard/diagnostics-graph";
import { Button, Tabs } from "#/components/ui";
import type { MeasurementT } from "#/providers/telemetry/telemetry/measurement";

const serializeMeasurement = (m: MeasurementT): string => {
	return JSON.stringify(
		m,
		(_key, value) => (typeof value === "bigint" ? value.toString() : value),
		2,
	);
};

const formatNanosDate = (nanos: bigint | number): string => {
	const n =
		typeof nanos === "bigint"
			? Number(nanos / 1_000_000n)
			: Number(nanos) / 1_000_000;
	if (!n || n <= 0) return "—";
	const d = new Date(n);
	if (isNaN(d.getTime())) return "—";
	return d.toISOString().replace("T", " ").replace("Z", "");
};

const formatMetricVal = (num: number): string => {
	if (!Number.isFinite(num)) return "—";
	if (Number.isInteger(num)) return num.toLocaleString();
	const abs = Math.abs(num);
	if (abs !== 0 && abs < 0.001) return num.toExponential(3);
	return num.toFixed(4);
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
			className={`mt-0.5 min-h-lh font-mono text-[11px] tabular-nums ${tone}`}
		>
			{value}
		</div>
	</div>
);

const MeasurementView = ({
	measurement,
	onSelectStage,
}: {
	measurement: MeasurementT | null | undefined;
	onSelectStage: (name: string) => void;
}) => {
	if (!measurement) {
		return (
			<div className="flex flex-col items-center justify-center py-8 text-center">
				<span className="size-2 rounded-full bg-(--warn) animate-pulse mb-2" />
				<div className="font-mono text-[11px] text-(--f2)">
					No measurement captured yet
				</div>
				<div className="font-mono text-[9px] text-(--f4) mt-1">
					Waiting for a live measurement frame from this stage...
				</div>
			</div>
		);
	}

	const metrics = measurement.metrics ?? [];
	const metadata = measurement.metadata ?? [];
	const provenance = measurement.provenance ?? [];
	const peers = measurement.peers ?? [];

	return (
		<div className="space-y-3 py-2">
			{/* Identity & Core Status */}
			<div className="grid grid-cols-2 gap-x-3">
				<Metric
					label="Tick"
					value={measurement.tick ? `#${measurement.tick.toString()}` : "—"}
					tone="text-(--acc)"
				/>
				<Metric
					label="Symbol"
					value={(typeof measurement.symbol === "string" && measurement.symbol) || "pipeline"}
				/>
				<Metric
					label="SNR"
					value={
						measurement.snrDefined && Number.isFinite(measurement.snr)
							? `${measurement.snr.toFixed(2)} dB`
							: "—"
					}
					tone={measurement.snrDefined && measurement.snr > 0 ? "text-(--up)" : "text-(--f1)"}
				/>
				<Metric
					label="Maturity"
					value={
						measurement.maturity > 0
							? `${(measurement.maturity * 100).toFixed(1)}%`
							: "—"
					}
				/>
				<Metric
					label="Timestamp (At)"
					value={formatNanosDate(measurement.at)}
				/>
				<Metric
					label="Horizon"
					value={
						measurement.horizon > 0n
							? formatNanos(Number(measurement.horizon))
							: "—"
					}
				/>
			</div>

			{measurement.id ? (
				<div className="border-(--line) border-b pb-2">
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						ID
					</div>
					<div className="mt-0.5 font-mono text-[10px] text-(--f3) break-all">
						{typeof measurement.id === "string" ? measurement.id : String(measurement.id)}
					</div>
				</div>
			) : null}

			{/* Metrics Table */}
			<div className="border-(--line) border-b pb-2">
				<div className="flex items-center justify-between mb-1.5">
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						Metrics ({metrics.length})
					</div>
				</div>
				{metrics.length === 0 ? (
					<div className="font-mono text-[9px] text-(--f4)">No metrics attached</div>
				) : (
					<div className="space-y-1.5">
						{metrics.map((m, idx) => {
							const name = typeof m.name === "string" ? m.name : String(m.name ?? idx);
							const rawVal = formatMetricVal(m.raw);
							const hasNorm = m.hasNormalized && Number.isFinite(m.normalized);
							const normVal = hasNorm ? Math.min(1, Math.max(0, m.normalized)) : 0;
							const unit = typeof m.unit === "string" ? m.unit : "";

							return (
								<div
									key={name + idx}
									className="rounded bg-(--sunken) p-1.5 font-mono text-[10px] border border-(--line)"
								>
									<div className="flex items-center justify-between">
										<span className="font-bold text-(--f1)">{name}</span>
										<span className="tabular-nums text-(--acc)">
											{rawVal} {unit ? <span className="text-(--f4) text-[9px]">{unit}</span> : null}
										</span>
									</div>
									{hasNorm ? (
										<div className="mt-1 flex items-center gap-2">
											<div className="h-1 flex-1 rounded bg-(--line) overflow-hidden">
												<div
													className="h-full bg-(--acc) transition-all duration-300"
													style={{ width: `${(normVal * 100).toFixed(0)}%` }}
												/>
											</div>
											<span className="text-[8px] text-(--f4) tabular-nums">
												{(normVal * 100).toFixed(0)}%
											</span>
										</div>
									) : null}
									{m.region > 0 ? (
										<div className="mt-1 text-[8px] text-(--f4)">
											region: <span className="text-(--f2)">{m.region}</span>
											{m.x > 0n || m.y > 0n ? ` · (${m.x}, ${m.y})` : ""}
										</div>
									) : null}
								</div>
							);
						})}
					</div>
				)}
			</div>

			{/* Metadata & Provenance */}
			{metadata.length > 0 || provenance.length > 0 ? (
				<div className="border-(--line) border-b pb-2">
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4) mb-1.5">
						Metadata & Provenance
					</div>
					<div className="flex flex-wrap gap-1">
						{metadata.map((entry, idx) => (
							<span
								key={idx}
								className="rounded border border-(--line) bg-(--sunken) px-1.5 py-0.5 font-mono text-[9px] text-(--f3)"
							>
								<span className="text-(--f4)">{entry.name}:</span> {entry.value}
							</span>
						))}
						{provenance.map((entry, idx) => (
							<span
								key={idx}
								className="rounded border border-(--line) bg-(--sunken) px-1.5 py-0.5 font-mono text-[9px] text-(--f3)"
							>
								<span className="text-(--f4)">{entry.name}:</span> {entry.value}
							</span>
						))}
					</div>
				</div>
			) : null}

			{/* Attached Peers */}
			{peers.length > 0 ? (
				<div className="border-(--line) border-b pb-2">
					<div className="font-mono text-[8px] uppercase tracking-widest text-(--f4) mb-1.5">
						Attached Peers ({peers.length})
					</div>
					<div className="flex flex-wrap gap-1">
						{peers.map((peer, idx) => {
							const peerSource = typeof peer.source === "string" ? peer.source : "";
							return (
								<Button
									key={idx}
									variant="outline"
									size="xxs"
									onClick={() => peerSource && onSelectStage(peerSource)}
								>
									{peerSource || "peer"}
								</Button>
							);
						})}
					</div>
				</div>
			) : null}
		</div>
	);
};

const MeasurementJsonView = ({
	measurement,
}: {
	measurement: MeasurementT | null | undefined;
}) => {
	const [copied, setCopied] = useState(false);

	if (!measurement) {
		return (
			<div className="flex flex-col items-center justify-center py-8 text-center">
				<span className="size-2 rounded-full bg-(--warn) animate-pulse mb-2" />
				<div className="font-mono text-[11px] text-(--f2)">
					No measurement captured yet
				</div>
			</div>
		);
	}

	const json = serializeMeasurement(measurement);

	const handleCopy = () => {
		navigator.clipboard.writeText(json);
		setCopied(true);
		setTimeout(() => setCopied(false), 2000);
	};

	return (
		<div className="py-2 space-y-2">
			<div className="flex items-center justify-between">
				<span className="font-mono text-[8px] uppercase tracking-widest text-(--f4)">
					Wire Representation (JSON)
				</span>
				<Button variant="outline" size="xxs" onClick={handleCopy}>
					{copied ? "Copied!" : "Copy JSON"}
				</Button>
			</div>
			<pre className="max-h-[60vh] overflow-auto rounded border border-(--line) bg-(--sunken) p-2 font-mono text-[9px] leading-tight text-(--f2) select-all">
				{json}
			</pre>
		</div>
	);
};

const StageDetail = ({
	name,
	stage,
	nodes,
	edges,
	activeTab,
	onTabChange,
	onSelect,
}: {
	name: string;
	stage: NodeStats | undefined;
	nodes: Map<string, NodeStats>;
	edges: EdgeStats[];
	activeTab: "overview" | "measurement" | "json";
	onTabChange: (tab: "overview" | "measurement" | "json") => void;
	onSelect: (selection: DiagnosticsSelection) => void;
}) => {
	// Only real hops. A stamp from a concurrent sibling is the same barrier
	// reporting itself in race order, so listing those as feeders would claim
	// a dependency between stages that simply ran at the same time.
	const hops = edges.filter((edge) => isHop(nodes, edge));
	const feeds = hops.filter((edge) => edge.from === name);
	const fedBy = hops.filter((edge) => edge.to === name);
	const siblings = Array.from(nodes.values()).filter(
		(other) => other.label !== name && concurrentSiblings(stage, other),
	);

	return (
		<>
			<div className="border-(--line) border-b px-3 py-2.5">
				<div className="flex items-center gap-2">
					<span
						className={`size-2 rounded-full ${stage ? "bg-(--up)" : "bg-(--line2)"}`}
					/>
					<span className="font-mono text-[12px] font-bold text-(--f1)">
						{name}
					</span>
					<span className="ml-auto font-mono text-[8px] uppercase text-(--f4)">
						{stage ? "observed" : "unseen"}
					</span>
				</div>
				{stage !== undefined && stage.group !== "" ? (
					<button
						type="button"
						onClick={() => onSelect({ kind: "group", name: stage.group })}
						className="mt-1.5 font-mono text-[9px] uppercase tracking-widest text-(--f4) hover:text-(--acc)"
					>
						ring {stage.group} · stage {stage.stage}
					</button>
				) : null}
			</div>

			<div className="border-(--line) border-b px-3 py-1.5 bg-(--sunken)">
				<Tabs size="xs" fullWidth>
					<Tabs.Tab
						active={activeTab === "overview"}
						onClick={() => onTabChange("overview")}
						grow
					>
						Overview
					</Tabs.Tab>
					<Tabs.Tab
						active={activeTab === "measurement"}
						onClick={() => onTabChange("measurement")}
						grow
					>
						Measurement {stage?.lastMeasurement?.metrics?.length ? `(${stage.lastMeasurement.metrics.length})` : ""}
					</Tabs.Tab>
					<Tabs.Tab
						active={activeTab === "json"}
						onClick={() => onTabChange("json")}
						grow
					>
						JSON
					</Tabs.Tab>
				</Tabs>
			</div>

			<div className="min-h-0 flex-1 overflow-auto px-3">
				{activeTab === "overview" ? (
					<>
						<div className="grid grid-cols-2 gap-x-3">
							<Metric
								label="rate"
								value={stage ? formatRate(stage.avgGapNs) : "—"}
							/>
							<Metric label="last gap" value={formatNanos(stage?.lastGapNs)} />
							<Metric label="average gap" value={formatNanos(stage?.avgGapNs)} />
							<Metric
								label="lifetime calls"
								value={(stage?.seqCount ?? 0).toLocaleString()}
							/>
							<Metric
								label="ring backlog"
								value={(stage?.backlog ?? 0).toLocaleString()}
								tone={(stage?.backlog ?? 0) > 0 ? "text-(--warn)" : "text-(--f1)"}
							/>
							<Metric
								label="session peak backlog"
								value={(stage?.maxBacklog ?? 0).toLocaleString()}
							/>
						</div>
						<div className="border-(--line) border-b py-2">
							<div className="mb-1 font-mono text-[8px] uppercase tracking-widest text-(--f4)">
								fed by
							</div>
							<div className="flex flex-wrap gap-1">
								{fedBy.map((edge) => (
									<Button
										key={edge.from}
										variant="outline"
										size="xxs"
										onClick={() => onSelect({ kind: "stage", name: edge.from })}
									>
										{edge.from} · {formatNanos(edge.avgLatencyNs)}
									</Button>
								))}
								{fedBy.length === 0 ? (
									<span className="font-mono text-[9px] text-(--f4)">
										source stage
									</span>
								) : null}
							</div>
						</div>
						<div className="border-(--line) border-b py-2">
							<div className="mb-1 font-mono text-[8px] uppercase tracking-widest text-(--f4)">
								feeds
							</div>
							<div className="flex flex-wrap gap-1">
								{feeds.map((edge) => (
									<Button
										key={edge.to}
										variant="outline"
										size="xxs"
										onClick={() => onSelect({ kind: "stage", name: edge.to })}
									>
										{edge.to} · {formatNanos(edge.avgLatencyNs)}
									</Button>
								))}
								{feeds.length === 0 ? (
									<span className="font-mono text-[9px] text-(--f4)">
										terminal stage
									</span>
								) : null}
							</div>
						</div>
						{siblings.length > 0 ? (
							<div className="border-(--line) border-b py-2">
								<div className="mb-1 font-mono text-[8px] uppercase tracking-widest text-(--f4)">
									runs alongside
								</div>
								<div className="flex flex-wrap gap-1">
									{siblings.map((sibling) => (
										<Button
											key={sibling.label}
											variant="outline"
											size="xxs"
											onClick={() =>
												onSelect({ kind: "stage", name: sibling.label })
											}
										>
											{sibling.label}
										</Button>
									))}
								</div>
								<div className="mt-1 font-mono text-[9px] leading-relaxed text-(--f4)">
									Same handler group, same barrier — these run concurrently against
									the same envelope, so nothing orders them against each other.
								</div>
							</div>
						) : null}
					</>
				) : activeTab === "measurement" ? (
					<MeasurementView
						measurement={stage?.lastMeasurement}
						onSelectStage={(target) => onSelect({ kind: "stage", name: target })}
					/>
				) : (
					<MeasurementJsonView measurement={stage?.lastMeasurement} />
				)}
			</div>
		</>
	);
};

/*
GroupDetail describes one runtime ring: the stages it owns, in the barrier
order the ring itself reported, and what crosses its boundary. The stage list
is grouped by handler group rather than flattened, because that is the fact a
trace of hops cannot recover.
*/
const GroupDetail = ({
	name,
	nodes,
	edges,
	onSelect,
}: {
	name: string;
	nodes: Map<string, NodeStats>;
	edges: EdgeStats[];
	onSelect: (selection: DiagnosticsSelection) => void;
}) => {
	const members = Array.from(nodes.values()).filter(
		(node) => node.group === name,
	);
	const labels = new Set(members.map((member) => member.label));
	const hops = edges.filter((edge) => isHop(nodes, edge));
	const inbound = hops.filter(
		(edge) => !labels.has(edge.from) && labels.has(edge.to),
	);
	const outbound = hops.filter(
		(edge) => labels.has(edge.from) && !labels.has(edge.to),
	);

	const byStage = new Map<number, NodeStats[]>();

	for (const member of members) {
		byStage.set(member.stage, [...(byStage.get(member.stage) ?? []), member]);
	}

	const stages = [...byStage.keys()].sort((a, b) => a - b);

	return (
		<>
			<div className="border-(--line) border-b px-3 py-2.5">
				<div className="flex items-center gap-2">
					<span className="size-2 rounded-full bg-(--acc)" />
					<span className="font-mono text-[12px] font-bold text-(--f1)">
						{name}
					</span>
					<span className="ml-auto font-mono text-[8px] uppercase text-(--f4)">
						ring
					</span>
				</div>
				<div className="mt-1 font-mono text-[9px] uppercase tracking-widest text-(--f4)">
					{members.length} stages · {stages.length} barriers
				</div>
			</div>
			<div className="min-h-0 flex-1 overflow-auto px-3">
				{stages.map((stage) => (
					<div key={stage} className="border-(--line) border-b py-2">
						<div className="mb-1 font-mono text-[8px] uppercase tracking-widest text-(--f4)">
							stage {stage}
							{(byStage.get(stage)?.length ?? 0) > 1 ? " · concurrent" : ""}
						</div>
						<div className="flex flex-wrap gap-1">
							{(byStage.get(stage) ?? []).map((member) => (
								<Button
									key={member.label}
									variant="outline"
									size="xxs"
									onClick={() =>
										onSelect({ kind: "stage", name: member.label })
									}
								>
									{member.label} · {formatRate(member.avgGapNs)}
								</Button>
							))}
						</div>
					</div>
				))}
				<div className="border-(--line) border-b py-2">
					<div className="mb-1 font-mono text-[8px] uppercase tracking-widest text-(--f4)">
						crosses the boundary
					</div>
					<div className="font-mono text-[9px] leading-relaxed text-(--f3)">
						{inbound.length} in · {outbound.length} out
					</div>
				</div>
			</div>
		</>
	);
};

const OverviewDetail = ({
	nodeCount,
	groupCount,
	hopCount,
}: {
	nodeCount: number;
	groupCount: number;
	hopCount: number;
}) => (
	<>
		<div className="border-(--line) border-b px-3 py-2.5 font-mono text-[12px] font-bold text-(--f1)">
			Inspection
		</div>
		<div className="px-3 py-3 font-mono text-[9px] leading-relaxed text-(--f3)">
			Select a stage to see who feeds it (amber) and who it feeds (blue), or a
			ring to see everything running inside it. Every node, hop and ring on this
			graph is discovered live — the hops from the boundary stamps each envelope
			carries, the rings from the runtime composition itself. Nothing here is a
			hand-maintained diagram.
		</div>
		<div className="border-(--line) border-b px-3 py-2.5 font-mono text-[12px] font-bold text-(--f1)">
			Topology <span className="text-(--acc)">{nodeCount} stages</span>
		</div>
		<div className="px-3 py-2 font-mono text-[9px] leading-relaxed text-(--f3)">
			{groupCount} rings · {hopCount} hops observed
		</div>
	</>
);

const DetailPanel = ({
	nodes,
	edges,
	selection,
	activeTab,
	onTabChange,
	onSelect,
}: {
	nodes: Map<string, NodeStats>;
	edges: EdgeStats[];
	selection: DiagnosticsSelection | null;
	activeTab: "overview" | "measurement" | "json";
	onTabChange: (tab: "overview" | "measurement" | "json") => void;
	onSelect: (selection: DiagnosticsSelection) => void;
}) => {
	if (selection?.kind === "stage") {
		return (
			<StageDetail
				name={selection.name}
				stage={nodes.get(selection.name)}
				nodes={nodes}
				edges={edges}
				activeTab={activeTab}
				onTabChange={onTabChange}
				onSelect={onSelect}
			/>
		);
	}

	if (selection?.kind === "group") {
		return (
			<GroupDetail
				name={selection.name}
				nodes={nodes}
				edges={edges}
				onSelect={onSelect}
			/>
		);
	}

	return (
		<OverviewDetail
			nodeCount={nodes.size}
			groupCount={
				new Set(Array.from(nodes.values()).map((node) => node.group)).size
			}
			hopCount={edges.filter((edge) => isHop(nodes, edge)).length}
		/>
	);
};

const Legend = () => (
	<div className="flex h-full flex-wrap items-center gap-x-3 gap-y-1 px-3 font-mono text-[8px] uppercase tracking-wide text-(--f4)">
		<span className="flex items-center gap-1.5">
			<span className="size-1.5 rounded-full bg-(--up)" /> live
		</span>
		<span className="flex items-center gap-1.5">
			<span className="size-1.5 rounded-full bg-(--f4)" /> stale
		</span>
		<span className="border-l border-(--line) mx-0.5 h-3" aria-hidden="true" />
		<span className="flex items-center gap-1.5">
			<span className="h-0.5 w-4 rounded bg-(--up)" /> healthy latency
		</span>
		<span className="flex items-center gap-1.5">
			<span className="h-0.5 w-4 rounded bg-(--warn)" /> slight latency
		</span>
		<span className="flex items-center gap-1.5">
			<span className="h-0.5 w-4 rounded bg-(--down)" /> high latency
		</span>
		<span className="border-l border-(--line) mx-0.5 h-3" aria-hidden="true" />
		<span className="flex items-center gap-1.5">
			<span className="h-0.5 w-4 animate-pulse rounded bg-(--acc)" /> flowing
		</span>
		<span className="flex items-center gap-1.5">
			<span className="h-0.5 w-4 rounded bg-(--f3)" /> idle
		</span>
		<span className="border-l border-(--line) mx-0.5 h-3" aria-hidden="true" />
		<span className="flex items-center gap-1.5">
			<span className="size-1.5 rounded-full bg-(--down)" /> ring backed up
		</span>
	</div>
);

const StatusStrip = ({
	nodeCount,
	groupCount,
	edgeCount,
	liveCount,
	backedUpCount,
	connection,
}: {
	nodeCount: number;
	groupCount: number;
	edgeCount: number;
	liveCount: number;
	backedUpCount: number;
	connection: "ONLINE" | "OFFLINE" | "CONNECTING";
}) => (
	<div className="flex h-8 shrink-0 items-center gap-4 border-(--line) border-b px-3 font-mono text-[9px] uppercase tracking-wide text-(--f4)">
		<span className={connection === "ONLINE" ? "text-(--up)" : "text-(--warn)"}>
			● {connection === "ONLINE" ? "wired" : connection.toLowerCase()}
		</span>
		<span>
			stages{" "}
			<strong className="inline-block w-[2ch] text-right tabular-nums text-(--up)">
				{formatCount(liveCount)}
			</strong>{" "}
			/ {nodeCount}
		</span>
		<span>
			rings{" "}
			<strong className="inline-block w-[2ch] text-right tabular-nums text-(--acc)">
				{formatCount(groupCount)}
			</strong>
		</span>
		<span>
			hops{" "}
			<strong className="inline-block w-[3ch] text-right tabular-nums text-(--info)">
				{formatCount(edgeCount)}
			</strong>
		</span>
		{backedUpCount > 0 ? (
			<span>
				backed up{" "}
				<strong className="inline-block w-[2ch] text-right tabular-nums text-(--down)">
					{formatCount(backedUpCount)}
				</strong>
			</span>
		) : null}
	</div>
);

/*
DiagnosticsSurface renders the live pipeline topology and per-stage detail,
sourced entirely from the topologyStore that websocket.tsx feeds off every
envelope's Boundaries stamps — the same socket every other measurement
already rides, so this page adds no extra traffic of its own.
*/
const DiagnosticsSurface = () => {
	// nodes/edges are Maps mutated in place by ingest() (see topology.ts) —
	// selecting them directly would return the same reference every render, so
	// the store's re-render trigger is `version` (a primitive that changes on
	// every ingest) and the Maps are read fresh off current state each render.
	useStore(topologyStore, (state) => state.version);
	const { nodes, edges } = topologyStore.state;
	const connection = useSelector(onlineAtom, (s) => s);
	const [selection, setSelection] = useState<DiagnosticsSelection | null>(null);
	const [activeTab, setActiveTab] = useState<"overview" | "measurement" | "json">(
		"overview",
	);

	const edgeList = Array.from(edges.values());
	const atNs = Math.max(
		0,
		...Array.from(nodes.values()).map((node) => node.lastAtNs),
		...edgeList.map((edge) => edge.lastAtNs),
	);
	const liveCount = Array.from(nodes.values()).filter(
		(node) => atNs - node.lastAtNs <= 2_000_000_000,
	).length;
	const backedUpCount = Array.from(nodes.values()).filter(
		(node) => backlogTone(node.backlog, node.maxBacklog) === "backed-up",
	).length;
	const groupCount = new Set(
		Array.from(nodes.values()).map((node) => node.group),
	).size;
	// Count what the graph actually draws: a stamp pair from one handler group
	// is two siblings finishing in some order, not a hop between them.
	const hopCount = edgeList.filter((edge) => isHop(nodes, edge)).length;

	return (
		<div className="grid h-full max-h-[calc(100dvh-3.25rem)] min-h-0 min-w-230 grid-cols-[minmax(0,1fr)_minmax(280px,22vw)] overflow-hidden bg-(--bg)">
			<section className="flex min-h-0 min-w-0 flex-col overflow-hidden border-(--line) border-r">
				<StatusStrip
					nodeCount={nodes.size}
					groupCount={groupCount}
					edgeCount={hopCount}
					liveCount={liveCount}
					backedUpCount={backedUpCount}
					connection={connection}
				/>
				<div className="flex h-6 shrink-0 items-center border-(--line) border-b bg-(--surface)">
					<Legend />
				</div>
				<div className="min-h-0 flex-1 overflow-hidden p-3">
					<DiagnosticsGraph
						nodes={nodes}
						edges={edges}
						atNs={atNs}
						selection={selection}
						onSelect={setSelection}
					/>
				</div>
			</section>
			<aside className="flex min-h-0 flex-col overflow-hidden bg-(--surface)">
				<DetailPanel
					nodes={nodes}
					edges={edgeList}
					selection={selection}
					activeTab={activeTab}
					onTabChange={setActiveTab}
					onSelect={setSelection}
				/>
			</aside>
		</div>
	);
};

export const Route = createFileRoute("/diagnostics")({
	component: DiagnosticsSurface,
});
