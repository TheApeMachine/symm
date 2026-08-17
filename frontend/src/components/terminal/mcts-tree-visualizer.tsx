import { useMemo, useState } from "react";
import { useSelector } from "@tanstack/react-store";
import { decisionStore } from "#/collections/decisions";
import type { DecisionMCTSTreeNode } from "#/types/thesis";
import { Input } from "@/components/ui/input";
import { Typography } from "@/components/ui/typography";

type MCTSTreeVisualizerProps = {
	symbol: string;
	className?: string;
};

const finite = (value: number | undefined, digits = 4): string => {
	if (value === undefined || !Number.isFinite(value)) {
		return "—";
	}

	return value.toFixed(digits);
};

const rewardTone = (value: number): string => {
	if (value > 0.001) {
		return "text-(--up)";
	}

	if (value < -0.001) {
		return "text-(--down)";
	}

	return "text-(--f3)";
};

const rewardBg = (value: number): string => {
	if (value > 0.001) {
		return "bg-[color-mix(in_srgb,var(--up)_15%,transparent)] border-[color-mix(in_srgb,var(--up)_35%,transparent)]";
	}

	if (value < -0.001) {
		return "bg-[color-mix(in_srgb,var(--down)_15%,transparent)] border-[color-mix(in_srgb,var(--down)_35%,transparent)]";
	}

	return "bg-(--sunken) border-(--line)";
};

const hasMatchingNode = (node: DecisionMCTSTreeNode, query: string): boolean => {
	if (query === "") {
		return true;
	}

	const needle = query.toLowerCase();

	if (node.actionName.toLowerCase().includes(needle) || (node.scmReason?.toLowerCase().includes(needle) ?? false)) {
		return true;
	}

	return node.children?.some((child) => hasMatchingNode(child, query)) ?? false;
};

const BranchNodeView = ({
	node,
	parentVisits,
	isRoot = false,
	selectedNodeId,
	onSelectNode,
	filterQuery,
	expandedMap,
	onToggleExpand,
}: {
	node: DecisionMCTSTreeNode;
	parentVisits: number;
	isRoot?: boolean;
	selectedNodeId: string | null;
	onSelectNode: (node: DecisionMCTSTreeNode) => void;
	filterQuery: string;
	expandedMap: Map<string, boolean>;
	onToggleExpand: (nodeKey: string) => void;
}) => {
	const nodeKey = `${node.actionName}:${node.depth}:${node.action}`;
	const isExpanded = expandedMap.get(nodeKey) ?? (isRoot || node.selected || node.depth < 2);
	const hasChildren = (node.children?.length ?? 0) > 0;
	const isNodeSelected = selectedNodeId === nodeKey;

	const visitShare = parentVisits > 0 ? (node.visits / parentVisits) * 100 : 100;
	const exploitShare = node.selectionScore > 0 && node.exploitation > 0
		? Math.min(100, (node.exploitation / node.selectionScore) * 100)
		: 50;

	if (filterQuery !== "" && !hasMatchingNode(node, filterQuery)) {
		return null;
	}

	return (
		<div className="relative min-w-0">
			<div
				className={`group relative rounded-md border p-3 font-mono text-[11px] transition-all ${
					isNodeSelected
						? "border-(--acc) bg-[color-mix(in_srgb,var(--acc)_12%,transparent)] shadow-[0_0_12px_rgba(235,140,50,0.15)]"
						: node.selected
							? "border-[color-mix(in_srgb,var(--up)_60%,transparent)] bg-[color-mix(in_srgb,var(--up)_8%,transparent)]"
							: rewardBg(node.meanReward)
				}`}
			>
				{/* Top row: depth, action name, badges */}
				<div className="flex flex-wrap items-center justify-between gap-2">
					<div className="flex min-w-0 items-center gap-2">
						{hasChildren && (
							<button
								type="button"
								onClick={(e) => {
									e.stopPropagation();
									onToggleExpand(nodeKey);
								}}
								className="flex size-4.5 cursor-pointer items-center justify-center rounded border border-(--line) bg-(--raised) text-[10px] text-(--f3) hover:border-(--line2) hover:text-(--f1)"
							>
								{isExpanded ? "−" : "+"}
							</button>
						)}
						<span className="rounded bg-(--raised) px-1.5 py-0.5 text-[9px] uppercase tracking-wider text-(--f4)">
							d{node.depth}
						</span>
						<button
							type="button"
							onClick={() => onSelectNode(node)}
							className="truncate text-left font-semibold text-[11.5px] text-(--f1) transition-colors hover:text-(--acc)"
							title={node.actionName}
						>
							{isRoot ? "Root Search Origin" : node.actionName}
						</button>
					</div>

					<div className="flex items-center gap-1.5">
						{node.selected && (
							<span className="rounded border border-[color-mix(in_srgb,var(--up)_60%,transparent)] bg-[color-mix(in_srgb,var(--up)_15%,transparent)] px-1.5 py-0.5 text-[8.5px] uppercase tracking-wider text-(--up)">
								Selected Path
							</span>
						)}
						{node.principal && !node.selected && (
							<span className="rounded border border-(--line2) bg-(--raised) px-1.5 py-0.5 text-[8.5px] uppercase tracking-wider text-(--f2)">
								Principal
							</span>
						)}
						<button
							type="button"
							onClick={() => onSelectNode(node)}
							className="cursor-pointer text-[9px] text-(--f4) hover:text-(--f1)"
						>
							Inspect →
						</button>
					</div>
				</div>

				{/* Metrics Grid */}
				<div className="mt-2.5 grid grid-cols-2 gap-x-4 gap-y-1.5 text-[10px] sm:grid-cols-4">
					<div className="flex justify-between gap-1 border-(--line) border-b pb-1">
						<span className="text-(--f4)">Visits</span>
						<span className="font-semibold text-(--f1)">
							{node.visits}{" "}
							<span className="text-[9px] text-(--f4)">({visitShare.toFixed(0)}%)</span>
						</span>
					</div>

					<div className="flex justify-between gap-1 border-(--line) border-b pb-1">
						<span className="text-(--f4)">Mean Reward</span>
						<span className={`font-semibold ${rewardTone(node.meanReward)}`}>
							{finite(node.meanReward, 4)}
						</span>
					</div>

					<div className="flex justify-between gap-1 border-(--line) border-b pb-1">
						<span className="text-(--f4)">UCT Score</span>
						<span className="text-(--f2)">{finite(node.selectionScore, 4)}</span>
					</div>

					<div className="flex justify-between gap-1 border-(--line) border-b pb-1">
						<span className="text-(--f4)">do(Action)</span>
						<span className={rewardTone(node.causalExpectation)}>
							{finite(node.causalExpectation, 4)}
						</span>
					</div>
				</div>

				{/* Visual Exploit vs Explore Gauge */}
				<div className="mt-2 flex items-center gap-2">
					<span className="text-[8.5px] uppercase text-(--f4)">UCT Split</span>
					<div className="relative h-1.5 flex-1 overflow-hidden rounded-full bg-(--sunken)">
						<div
							className="h-full bg-(--acc) transition-all duration-300"
							style={{ width: `${Math.max(4, Math.min(96, exploitShare))}%` }}
							title={`Exploitation: ${finite(node.exploitation)} / Exploration: ${finite(node.exploration)}`}
						/>
					</div>
					<div className="flex gap-2 text-[8.5px] text-(--f4)">
						<span className="text-(--acc)">exploit</span>
						<span>explore</span>
					</div>
				</div>
			</div>

			{/* Render Child Branches */}
			{isExpanded && hasChildren && (
				<div className="relative ml-4 mt-2 space-y-2 border-(--line2) border-l-2 pl-3">
					{node.children?.map((child) => (
						<BranchNodeView
							key={`${child.actionName}:${child.depth}:${child.action}`}
							node={child}
							parentVisits={node.visits}
							selectedNodeId={selectedNodeId}
							onSelectNode={onSelectNode}
							filterQuery={filterQuery}
							expandedMap={expandedMap}
							onToggleExpand={onToggleExpand}
						/>
					))}
				</div>
			)}
		</div>
	);
};

export const MCTSTreeVisualizer = ({ symbol, className }: MCTSTreeVisualizerProps) => {
	const [selectedNode, setSelectedNode] = useState<DecisionMCTSTreeNode | null>(null);
	const [filterQuery, setFilterQuery] = useState("");
	const [expandedMap, setExpandedMap] = useState<Map<string, boolean>>(new Map());

	// Retrieve newest decision for symbol from decisionStore
	const liveDecisions = useSelector(decisionStore, (state) => {
		const storeState = state as unknown as { decisions?: Record<string, { values: () => unknown[] }> };
		return storeState.decisions?.[symbol]?.values() ?? [];
	});

	const latestDecision = (liveDecisions && liveDecisions.length > 0
		? liveDecisions[liveDecisions.length - 1]
		: null) as { trace?: { mcts?: { tree?: DecisionMCTSTreeNode; iterations?: number; maxDepth?: number; totalNodes?: number; recommendedAction?: string } } } | null;

	const trace = latestDecision?.trace;
	const mcts = trace?.mcts;
	const treeRoot = mcts?.tree;

	// Build breadcrumb trail of the optimal selected path
	const selectedPath = useMemo(() => {
		if (!treeRoot) {
			return [];
		}

		const path: DecisionMCTSTreeNode[] = [];
		let current: DecisionMCTSTreeNode | undefined = treeRoot;

		while (current) {
			path.push(current);
			current = current.children?.find((child) => child.selected);
		}

		return path;
	}, [treeRoot]);

	const toggleExpand = (nodeKey: string) => {
		setExpandedMap((prev) => {
			const next = new Map(prev);
			next.set(nodeKey, !(next.get(nodeKey) ?? true));
			return next;
		});
	};

	const expandAll = () => {
		if (!treeRoot) {
			return;
		}

		const next = new Map<string, boolean>();
		const visit = (node: DecisionMCTSTreeNode) => {
			const key = `${node.actionName}:${node.depth}:${node.action}`;
			next.set(key, true);
			node.children?.forEach(visit);
		};

		visit(treeRoot);
		setExpandedMap(next);
	};

	const collapseAll = () => {
		if (!treeRoot) {
			return;
		}

		const next = new Map<string, boolean>();
		const visit = (node: DecisionMCTSTreeNode) => {
			const key = `${node.actionName}:${node.depth}:${node.action}`;
			next.set(key, false);
			node.children?.forEach(visit);
		};

		visit(treeRoot);
		next.set(`${treeRoot.actionName}:${treeRoot.depth}:${treeRoot.action}`, true);
		setExpandedMap(next);
	};

	if (!mcts || !treeRoot) {
		return (
			<div className={`flex h-full flex-col items-center justify-center p-8 text-center ${className ?? ""}`}>
				<div className="size-3 animate-ping rounded-full bg-(--acc) opacity-75" />
				<Typography.Mono size="s" tone="f2" className="mt-4 uppercase tracking-wider">
					MCTS / Pearl Causal Search Initializing
				</Typography.Mono>
				<Typography.Paragraph className="mt-1 max-w-sm font-mono text-[11px] text-(--f4)">
					Waiting for causal graph simulation iterations and do-calculus counterfactual updates for {symbol}...
				</Typography.Paragraph>
			</div>
		);
	}

	return (
		<div className={`flex h-full min-h-0 flex-col gap-3 p-4 ${className ?? ""}`}>
			{/* Top Summary Bar */}
			<div className="flex flex-wrap items-center justify-between gap-3 border-(--line) border-b pb-3 font-mono text-[11px]">
				<div className="flex flex-wrap items-center gap-3">
					<div>
						<span className="text-(--f4)">Simulations: </span>
						<span className="font-semibold text-(--f1)">{mcts.iterations ?? 0}</span>
					</div>
					<div>
						<span className="text-(--f4)">Max Depth: </span>
						<span className="font-semibold text-(--f1)">{mcts.maxDepth ?? "—"}</span>
					</div>
					<div>
						<span className="text-(--f4)">Explored Nodes: </span>
						<span className="font-semibold text-(--f1)">{mcts.totalNodes ?? "—"}</span>
					</div>
					<div>
						<span className="text-(--f4)">Recommended: </span>
						<span className="font-semibold text-(--acc) uppercase">
							{mcts.recommendedAction ?? "—"}
						</span>
					</div>
				</div>

				<div className="flex items-center gap-2">
					<Input.Search
						placeholder="Filter branches…"
						value={filterQuery}
						onChange={(e) => setFilterQuery(e.target.value)}
						className="h-7 w-38 text-[10px]"
					/>
					<button
						type="button"
						onClick={expandAll}
						className="cursor-pointer rounded border border-(--line) bg-(--raised) px-2 py-1 text-[9.5px] uppercase text-(--f3) hover:text-(--f1)"
					>
						Expand
					</button>
					<button
						type="button"
						onClick={collapseAll}
						className="cursor-pointer rounded border border-(--line) bg-(--raised) px-2 py-1 text-[9.5px] uppercase text-(--f3) hover:text-(--f1)"
					>
						Collapse
					</button>
				</div>
			</div>

			{/* Optimal Trajectory Breadcrumb Bar */}
			{selectedPath.length > 0 && (
				<div className="flex flex-wrap items-center gap-1.5 rounded border border-(--line) bg-(--sunken) px-3 py-2 font-mono text-[10px]">
					<span className="text-(--f4) uppercase tracking-wider">Optimal Trajectory:</span>
					{selectedPath.map((pathNode, index) => (
						<span key={`path-${pathNode.actionName}-${pathNode.depth}`} className="flex items-center gap-1.5">
							{index > 0 && <span className="text-(--f4)">→</span>}
							<button
								type="button"
								onClick={() => setSelectedNode(pathNode)}
								className="cursor-pointer rounded bg-(--raised) px-1.5 py-0.5 text-(--f1) hover:text-(--acc)"
							>
								{pathNode.actionName}
							</button>
						</span>
					))}
				</div>
			)}

			{/* Main Split: Tree Breakdown (Left) + Node Inspector (Right) */}
			<div className="grid min-h-0 flex-1 grid-cols-[minmax(0,1.4fr)_minmax(280px,0.85fr)] gap-3 overflow-hidden">
				{/* Tree Hierarchy Scroll Container */}
				<div className="min-h-0 overflow-y-auto pr-1">
					<BranchNodeView
						node={treeRoot}
						parentVisits={treeRoot.visits}
						isRoot
						selectedNodeId={selectedNode ? `${selectedNode.actionName}:${selectedNode.depth}:${selectedNode.action}` : null}
						onSelectNode={(node) => setSelectedNode(node)}
						filterQuery={filterQuery}
						expandedMap={expandedMap}
						onToggleExpand={toggleExpand}
					/>
				</div>

				{/* Node Details & State Inspector */}
				<div className="min-h-0 overflow-y-auto rounded-md border border-(--line) bg-(--surface) p-3 font-mono text-[11px]">
					<div className="border-(--line) border-b pb-2">
						<Typography.Label size="s" tone="f1" className="uppercase tracking-wider">
							Branch Inspector
						</Typography.Label>
						<Typography.Paragraph className="mt-0.5 font-mono text-[10px] text-(--f4)">
							{selectedNode ? selectedNode.actionName : "Click any branch node on the left to inspect its causal state"}
						</Typography.Paragraph>
					</div>

					{selectedNode ? (
						<div className="mt-3 space-y-3">
							<div className="space-y-1.5">
								<div className="flex justify-between border-(--line) border-b py-1">
									<span className="text-(--f4)">Action Index</span>
									<span className="text-(--f1)">{selectedNode.action}</span>
								</div>
								<div className="flex justify-between border-(--line) border-b py-1">
									<span className="text-(--f4)">Search Depth</span>
									<span className="text-(--f1)">d{selectedNode.depth}</span>
								</div>
								<div className="flex justify-between border-(--line) border-b py-1">
									<span className="text-(--f4)">Simulations / Visits</span>
									<span className="font-semibold text-(--f1)">{selectedNode.visits}</span>
								</div>
								<div className="flex justify-between border-(--line) border-b py-1">
									<span className="text-(--f4)">Effective Visits</span>
									<span className="text-(--f2)">{finite(selectedNode.effectiveVisits, 2)}</span>
								</div>
								{selectedNode.counterfactualMass !== undefined && (
									<div className="flex justify-between border-(--line) border-b py-1">
										<span className="text-(--f4)">Counterfactual Mass</span>
										<span className="text-(--f2)">{finite(selectedNode.counterfactualMass, 2)}</span>
									</div>
								)}
								<div className="flex justify-between border-(--line) border-b py-1">
									<span className="text-(--f4)">Total Reward</span>
									<span className={rewardTone(selectedNode.totalReward)}>
										{finite(selectedNode.totalReward, 6)}
									</span>
								</div>
								<div className="flex justify-between border-(--line) border-b py-1">
									<span className="text-(--f4)">Mean Reward</span>
									<span className={`font-semibold ${rewardTone(selectedNode.meanReward)}`}>
										{finite(selectedNode.meanReward, 6)}
									</span>
								</div>
								<div className="flex justify-between border-(--line) border-b py-1">
									<span className="text-(--f4)">UCT Exploitation</span>
									<span className="text-(--f2)">{finite(selectedNode.exploitation, 6)}</span>
								</div>
								<div className="flex justify-between border-(--line) border-b py-1">
									<span className="text-(--f4)">UCT Exploration</span>
									<span className="text-(--f2)">{finite(selectedNode.exploration, 6)}</span>
								</div>
								<div className="flex justify-between border-(--line) border-b py-1">
									<span className="text-(--f4)">Pearl E[Target|do(Action)]</span>
									<span className={`font-semibold ${rewardTone(selectedNode.causalExpectation)}`}>
										{finite(selectedNode.causalExpectation, 6)}
									</span>
								</div>
								<div className="flex justify-between border-(--line) border-b py-1">
									<span className="text-(--f4)">Composite UCT Score</span>
									<span className="font-semibold text-(--acc)">{finite(selectedNode.selectionScore, 6)}</span>
								</div>
							</div>

							{selectedNode.scmReason && (
								<div className="rounded border border-(--line) bg-(--sunken) p-2">
									<span className="block text-[9.5px] uppercase text-(--f4)">SCM Provenance</span>
									<span className="mt-0.5 block text-[10px] text-(--f2)">
										{selectedNode.scmReason}
									</span>
								</div>
							)}

							{selectedNode.state && Object.keys(selectedNode.state).length > 0 && (
								<div className="rounded border border-(--line) bg-(--sunken) p-2">
									<span className="block text-[9.5px] uppercase text-(--f4)">State Semantic Frame</span>
									<div className="mt-1 space-y-0.5">
										{Object.entries(selectedNode.state).map(([stateKey, stateVal]) => (
											<div key={stateKey} className="flex justify-between text-[9.5px]">
												<span className="text-(--f4)">{stateKey}</span>
												<span className={rewardTone(stateVal)}>{finite(stateVal, 4)}</span>
											</div>
										))}
									</div>
								</div>
							)}
						</div>
					) : (
						<div className="mt-8 text-center text-(--f4)">
							Select any branch on the left tree to inspect its causal trajectory and SCM metrics.
						</div>
					)}
				</div>
			</div>
		</div>
	);
};
