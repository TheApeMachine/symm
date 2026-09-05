import { useMemo, useState } from "react";
import type {
	ReasoningPayload,
	ReasoningSearchNode,
	ReasoningTier,
	ReasoningTopology,
} from "#/types/reasoning";

const TIER_ORDER: ReasoningTier[] = ["measurement", "field", "scm", "decision"];

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
		return "text-(--success)";
	}

	if (value < 0) {
		return "text-(--error)";
	}

	return "text-(--f2)";
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
		? "border-(--acc)/70 bg-(--acc)/10"
		: node.principal
			? "border-(--info)/50 bg-(--info)/5"
			: "border-(--line2)/70 bg-(--sunken)/40";

	return (
		<div className="relative min-w-0">
			<button
				type="button"
				onClick={() => setExpanded((current) => !current)}
				className={`w-full rounded-sm border px-3 py-2 text-left transition ${selectedClass}`}
			>
				<div className="flex items-center justify-between gap-3">
					<div className="flex min-w-0 items-center gap-2">
						<span className="font-mono text-[10px] uppercase tracking-[0.18em] text-(--f3)">
							h{node.depth}
						</span>
						<span className="truncate font-mono text-xs font-semibold uppercase text-(--f1)">
							{root ? "root" : node.actionName}
						</span>
						{node.selected && (
							<span className="rounded-sm border border-(--acc)/50 px-1.5 py-0.5 font-mono text-[9px] uppercase text-(--acc)">
								selected
							</span>
						)}
						{node.principal && !node.selected && (
							<span className="rounded-sm border border-(--info)/40 px-1.5 py-0.5 font-mono text-[9px] uppercase text-(--info)">
								principal
							</span>
						)}
					</div>
					<span className="font-mono text-[10px] text-(--f3)">
						{children.length > 0 ? (expanded ? "−" : "+") : "·"}
					</span>
				</div>

				<div className="mt-2 grid grid-cols-4 gap-x-3 gap-y-1 font-mono text-[10px]">
					<span className="text-(--f3)">visits</span>
					<span className="text-(--f1)">{node.visits}</span>
					<span className="text-(--f3)">mean</span>
					<span className={metricClass(node.meanReward)}>
						{finite(node.meanReward)}
					</span>
					<span className="text-(--f3)">UCT exploit</span>
					<span className={metricClass(node.exploitation)}>
						{finite(node.exploitation)}
					</span>
					<span className="text-(--f3)">explore</span>
					<span className="text-(--f1)">{finite(node.exploration)}</span>
					<span className="text-(--f3)">do-expect</span>
					<span className={metricClass(node.causalExpectation)}>
						{finite(node.causalExpectation)}
					</span>
					<span className="text-(--f3)">score</span>
					<span className={metricClass(node.selectionScore)}>
						{finite(node.selectionScore)}
					</span>
				</div>

				<div className="mt-2 h-1 overflow-hidden rounded-full bg-(--line)">
					<div
						className="h-full bg-(--success)/70"
						style={{
							width: `${Math.max(0, Math.min(100, observedShare * 100))}%`,
						}}
					/>
				</div>
				<div className="mt-1 flex justify-between font-mono text-[9px] uppercase text-(--f3)">
					<span>observed {node.visits}</span>
					<span>counterfactual {finite(node.counterfactualMass, 2)}</span>
				</div>

				<div className="mt-2 flex items-center justify-between gap-3 border-t border-(--line)/80 pt-2 font-mono text-[9px] uppercase">
					<span className={node.scmReady ? "text-(--success)" : "text-(--acc)"}>
						SCM {node.scmReady ? "ready" : "warming"}
					</span>
					<span className="truncate text-(--f3)" title={node.scmReason}>
						{node.scmReason ?? "no provenance note"}
					</span>
				</div>
			</button>

			{expanded && children.length > 0 && (
				<div className="ml-4 mt-2 space-y-2 border-l border-(--line2)/70 pl-3">
					{children.map((child) => (
						<SearchBranch
							key={`${child.action}:${child.depth}:${child.actionName}`}
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
		<div className="grid min-h-24 grid-cols-[10rem_minmax(0,1fr)] border-b border-(--line)/80 last:border-b-0">
			<div className="border-r border-(--line)/80 bg-(--sunken)/70 px-3 py-3 font-mono text-[10px] uppercase tracking-[0.16em] text-(--f3)">
				{TIER_LABELS[tier]}
			</div>
			<div className="flex min-w-0 flex-wrap content-start gap-2 p-3">
				{nodes.length === 0 && (
					<span className="font-mono text-[10px] uppercase text-(--f4)">
						No active nodes
					</span>
				)}
				{nodes.map((node) => (
					<div
						key={node.id}
						className={`min-w-40 max-w-64 flex-1 rounded-sm border px-2.5 py-2 ${
							node.derived
								? "border-(--info)/40 bg-(--info)/5"
								: "border-(--line2)/70 bg-(--surface)/40"
						}`}
						title={node.id}
					>
						<div className="flex items-start justify-between gap-2">
							<span className="truncate font-mono text-[10px] uppercase text-(--f1)">
								{node.label}
							</span>
							{node.role && (
								<span className="font-mono text-[8px] uppercase text-(--info)">
									{node.role}
								</span>
							)}
						</div>
						<div className="mt-2 flex justify-between font-mono text-[9px] text-(--f3)">
							<span className={metricClass(node.value)}>
								{finite(node.value)}
							</span>
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
	const search =
		explicitSearch ??
		reasoningPayload?.search ??
		reasoningPayload?.root ??
		null;
	const stateEntries = useMemo(
		() =>
			Object.entries(topology?.currentState ?? {}).sort(([left], [right]) =>
				left.localeCompare(right),
			),
		[topology],
	);

	if (!topology && !search) {
		return (
			<div
				className={`rounded-sm border border-(--line) bg-(--sunken)/50 p-4 ${className}`}
			>
				<p className="font-mono text-[10px] uppercase tracking-[0.16em] text-(--f4)">
					Reasoning trace unavailable
				</p>
			</div>
		);
	}

	return (
		<section
			className={`grid min-h-0 gap-3 xl:grid-cols-[minmax(0,1.25fr)_minmax(22rem,0.75fr)] ${className}`}
		>
			<div className="min-h-0 overflow-hidden rounded-sm border border-(--line) bg-(--sunken)/40">
				<div className="flex flex-wrap items-center justify-between gap-3 border-b border-(--line) px-3 py-2">
					<div>
						<h2 className="font-mono text-xs font-semibold uppercase tracking-[0.14em] text-(--f1)">
							Causal topology
						</h2>
						<p className="mt-1 font-mono text-[9px] uppercase text-(--f4)">
							Evidence DAG → SCM → intervention
						</p>
					</div>
					{topology && (
						<div className="flex items-center gap-3 font-mono text-[9px] uppercase text-(--f3)">
							<span
								className={topology.ready ? "text-(--success)" : "text-(--acc)"}
							>
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
						<div className="border-t border-(--line) p-3">
							<div className="mb-2 font-mono text-[9px] uppercase tracking-[0.16em] text-(--f4)">
								Current semantic frame
							</div>
							<div className="grid grid-cols-2 gap-x-4 gap-y-1 sm:grid-cols-3 lg:grid-cols-4">
								{stateEntries.map(([name, value]) => (
									<div
										key={name}
										className="flex justify-between gap-2 font-mono text-[9px]"
									>
										<span className="truncate text-(--f4)">{name}</span>
										<span className={metricClass(value)}>{finite(value)}</span>
									</div>
								))}
							</div>
							<p className="mt-3 font-mono text-[9px] text-(--f4)">
								{topology.reason}
							</p>
						</div>
					</>
				)}
			</div>

			<div className="min-h-0 overflow-hidden rounded-sm border border-(--line) bg-(--sunken)/40">
				<div className="border-b border-(--line) px-3 py-2">
					<h2 className="font-mono text-xs font-semibold uppercase tracking-[0.14em] text-(--f1)">
						Intervention search
					</h2>
					<p className="mt-1 font-mono text-[9px] uppercase text-(--f4)">
						Observed reward, counterfactual mass, UCT, and do-expectation
					</p>
				</div>
				<div className="max-h-[72vh] overflow-auto p-3">
					{search ? (
						<SearchBranch node={search} root />
					) : (
						<p className="font-mono text-[10px] uppercase text-(--f4)">
							No search has been published for this snapshot
						</p>
					)}
				</div>
			</div>
		</section>
	);
};
