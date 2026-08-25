import { useMemo, useState } from "react";
import type {
	ReasoningPayload,
	ReasoningSearchNode,
	ReasoningTier,
	ReasoningTopology,
} from "#/types/reasoning";

const TIER_ORDER: ReasoningTier[] = [
	"measurement",
	"field",
	"scm",
	"decision",
];

const TIER_LABELS: Record<ReasoningTier, string> = {
	measurement: "01 · Measurements",
	field: "02 · Fields & constraints",
	scm: "03 · Structural causal model",
	decision: "04 · Strategic propositions",
};

type ReasoningInspectorProps = {
	payload?: unknown;
	topology?: ReasoningTopology | null;
	search?: ReasoningSearchNode | null;
	className?: string;
};

const finite = (value: number | undefined, digits = 3) => {
	if (value === undefined || !Number.isFinite(value)) {
		return "—";
	}

	return value.toFixed(digits);
};

const percent = (value: number | undefined) => {
	if (value === undefined || !Number.isFinite(value)) {
		return "—";
	}

	return `${(value * 100).toFixed(1)}%`;
};

const metricClass = (value: number) => {
	if (value > 0) {
		return "text-emerald-300";
	}

	if (value < 0) {
		return "text-rose-300";
	}

	return "text-zinc-400";
};

const SearchBranch = ({
	node,
	root = false,
}: {
	node: ReasoningSearchNode;
	root?: boolean;
}) => {
	const [expanded, setExpanded] = useState(root || node.principal);
	const children = node.children ?? [];
	const provenanceTotal = node.visits + node.counterfactualMass;
	const observedShare = provenanceTotal > 0 ? node.visits / provenanceTotal : 0;
	const selectedClass = node.selected
		? "border-amber-300/70 bg-amber-300/10"
		: node.principal
			? "border-cyan-300/50 bg-cyan-300/5"
			: "border-zinc-700/70 bg-zinc-950/40";

	return (
		<div className="relative min-w-0">
			<button
				type="button"
				onClick={() => setExpanded((current) => !current)}
				className={`w-full rounded-sm border px-3 py-2 text-left transition ${selectedClass}`}
			>
				<div className="flex items-center justify-between gap-3">
					<div className="flex min-w-0 items-center gap-2">
						<span className="font-mono text-[10px] uppercase tracking-[0.18em] text-zinc-500">
							h{node.depth}
						</span>
						<span className="truncate font-mono text-xs font-semibold uppercase text-zinc-100">
							{root ? "root" : node.actionName}
						</span>
						{node.selected && (
							<span className="rounded-sm border border-amber-300/50 px-1.5 py-0.5 font-mono text-[9px] uppercase text-amber-200">
								selected
							</span>
						)}
						{node.principal && !node.selected && (
							<span className="rounded-sm border border-cyan-300/40 px-1.5 py-0.5 font-mono text-[9px] uppercase text-cyan-200">
								principal
							</span>
						)}
					</div>
					<span className="font-mono text-[10px] text-zinc-500">
						{children.length > 0 ? (expanded ? "−" : "+") : "·"}
					</span>
				</div>

				<div className="mt-2 grid grid-cols-4 gap-x-3 gap-y-1 font-mono text-[10px]">
					<span className="text-zinc-500">visits</span>
					<span className="text-zinc-200">{node.visits}</span>
					<span className="text-zinc-500">mean</span>
					<span className={metricClass(node.meanReward)}>{finite(node.meanReward)}</span>
					<span className="text-zinc-500">UCT exploit</span>
					<span className={metricClass(node.exploitation)}>{finite(node.exploitation)}</span>
					<span className="text-zinc-500">explore</span>
					<span className="text-zinc-200">{finite(node.exploration)}</span>
					<span className="text-zinc-500">do-expect</span>
					<span className={metricClass(node.causalExpectation)}>
						{finite(node.causalExpectation)}
					</span>
					<span className="text-zinc-500">score</span>
					<span className={metricClass(node.selectionScore)}>{finite(node.selectionScore)}</span>
				</div>

				<div className="mt-2 h-1 overflow-hidden rounded-full bg-zinc-800">
					<div
						className="h-full bg-emerald-300/70"
						style={{ width: `${Math.max(0, Math.min(100, observedShare * 100))}%` }}
					/>
				</div>
				<div className="mt-1 flex justify-between font-mono text-[9px] uppercase text-zinc-500">
					<span>observed {node.visits}</span>
					<span>counterfactual {finite(node.counterfactualMass, 2)}</span>
				</div>

				<div className="mt-2 flex items-center justify-between gap-3 border-t border-zinc-800/80 pt-2 font-mono text-[9px] uppercase">
					<span className={node.scmReady ? "text-emerald-300" : "text-amber-300"}>
						SCM {node.scmReady ? "ready" : "warming"}
					</span>
					<span className="truncate text-zinc-500" title={node.scmReason}>
						{node.scmReason ?? "no provenance note"}
					</span>
				</div>
			</button>

			{expanded && children.length > 0 && (
				<div className="ml-4 mt-2 space-y-2 border-l border-zinc-700/70 pl-3">
					{children.map((child) => (
						<SearchBranch
							key={`${child.action}:${child.depth}:${child.visits}:${child.actionName}`}
							node={child}
						/>
					))}
				</div>
			)}
		</div>
	);
};

const TopologyLane = ({
	tier,
	topology,
}: {
	tier: ReasoningTier;
	topology: ReasoningTopology;
}) => {
	const nodes = topology.nodes.filter((node) => node.tier === tier);

	return (
		<div className="grid min-h-24 grid-cols-[10rem_minmax(0,1fr)] border-b border-zinc-800/80 last:border-b-0">
			<div className="border-r border-zinc-800/80 bg-zinc-950/70 px-3 py-3 font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-500">
				{TIER_LABELS[tier]}
			</div>
			<div className="flex min-w-0 flex-wrap content-start gap-2 p-3">
				{nodes.length === 0 && (
					<span className="font-mono text-[10px] uppercase text-zinc-700">
						No active nodes
					</span>
				)}
				{nodes.map((node) => (
					<div
						key={node.id}
						className={`min-w-40 max-w-64 flex-1 rounded-sm border px-2.5 py-2 ${
							node.derived
								? "border-cyan-300/40 bg-cyan-300/5"
								: "border-zinc-700/70 bg-zinc-900/40"
						}`}
						title={node.id}
					>
						<div className="flex items-start justify-between gap-2">
							<span className="truncate font-mono text-[10px] uppercase text-zinc-200">
								{node.label}
							</span>
							{node.role && (
								<span className="font-mono text-[8px] uppercase text-cyan-300">
									{node.role}
								</span>
							)}
						</div>
						<div className="mt-2 flex justify-between font-mono text-[9px] text-zinc-500">
							<span className={metricClass(node.value)}>{finite(node.value)}</span>
							<span>conf {percent(node.confidence)}</span>
						</div>
					</div>
				))}
			</div>
		</div>
	);
};

export const ReasoningInspector = ({
	payload,
	topology: explicitTopology,
	search: explicitSearch,
	className = "",
}: ReasoningInspectorProps) => {
	const reasoningPayload = (payload ?? null) as ReasoningPayload | null;
	const topology = explicitTopology ?? reasoningPayload?.reasoning ?? null;
	const search = explicitSearch ?? reasoningPayload?.search ?? reasoningPayload?.root ?? null;
	const stateEntries = useMemo(
		() => Object.entries(topology?.currentState ?? {}).sort(([left], [right]) => left.localeCompare(right)),
		[topology],
	);

	if (!topology && !search) {
		return (
			<div className={`rounded-sm border border-zinc-800 bg-zinc-950/50 p-4 ${className}`}>
				<p className="font-mono text-[10px] uppercase tracking-[0.16em] text-zinc-600">
					Reasoning trace unavailable
				</p>
			</div>
		);
	}

	return (
		<section className={`grid min-h-0 gap-3 xl:grid-cols-[minmax(0,1.25fr)_minmax(22rem,0.75fr)] ${className}`}>
			<div className="min-h-0 overflow-hidden rounded-sm border border-zinc-800 bg-zinc-950/40">
				<div className="flex flex-wrap items-center justify-between gap-3 border-b border-zinc-800 px-3 py-2">
					<div>
						<h2 className="font-mono text-xs font-semibold uppercase tracking-[0.14em] text-zinc-100">
							Causal topology
						</h2>
						<p className="mt-1 font-mono text-[9px] uppercase text-zinc-600">
							Evidence DAG → SCM → intervention
						</p>
					</div>
					{topology && (
						<div className="flex items-center gap-3 font-mono text-[9px] uppercase text-zinc-500">
							<span className={topology.ready ? "text-emerald-300" : "text-amber-300"}>
								{topology.ready ? "search ready" : "evidence incomplete"}
							</span>
							<span>{topology.observedRows} observed rows</span>
							<span>h≤{topology.maximumHorizon}</span>
						</div>
					)}
				</div>

				{topology && (
					<>
						<div className="max-h-[58vh] overflow-auto">
							{TIER_ORDER.map((tier) => (
								<TopologyLane key={tier} tier={tier} topology={topology} />
							))}
						</div>
						<div className="border-t border-zinc-800 p-3">
							<div className="mb-2 font-mono text-[9px] uppercase tracking-[0.16em] text-zinc-600">
								Current semantic frame
							</div>
							<div className="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3 lg:grid-cols-4">
								{stateEntries.map(([name, value]) => (
									<div key={name} className="flex justify-between gap-2 font-mono text-[9px]">
										<span className="truncate text-zinc-600">{name}</span>
										<span className={metricClass(value)}>{finite(value)}</span>
									</div>
								))}
							</div>
							<p className="mt-3 font-mono text-[9px] text-zinc-600">{topology.reason}</p>
						</div>
					</>
				)}
			</div>

			<div className="min-h-0 overflow-hidden rounded-sm border border-zinc-800 bg-zinc-950/40">
				<div className="border-b border-zinc-800 px-3 py-2">
					<h2 className="font-mono text-xs font-semibold uppercase tracking-[0.14em] text-zinc-100">
						Intervention search
					</h2>
					<p className="mt-1 font-mono text-[9px] uppercase text-zinc-600">
						Observed reward, counterfactual mass, UCT, and do-expectation
					</p>
				</div>
				<div className="max-h-[72vh] overflow-auto p-3">
					{search ? (
						<SearchBranch node={search} root />
					) : (
						<p className="font-mono text-[10px] uppercase text-zinc-700">
							No search has been published for this snapshot
						</p>
					)}
				</div>
			</div>
		</section>
	);
};