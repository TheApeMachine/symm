import { createFileRoute } from "@tanstack/react-router";
import { useStore } from "@tanstack/react-store";
import { useEffect, useRef, useState } from "react";
import {
	type PlaybookBranch,
	pathIsActive,
	playbookStore,
	type WalkStep,
	type WalkTrace,
	walkStepForPath,
} from "#/collections/playbook";
import { cn } from "#/lib/utils";

type ConditionWire = {
	type?: string;
	left?: { subject?: Record<string, unknown> };
	right?: { subject?: Record<string, unknown> };
};

const subjectLabel = (subject: Record<string, unknown> | undefined): string => {
	if (!subject) {
		return "subject";
	}

	if (subject.type === "holding") {
		const holding = subject.holding;

		if (typeof holding === "object" && holding !== null) {
			return (holding as Record<string, unknown>).held ? "holding" : "flat";
		}
	}

	if (subject.type === "category") {
		const category = subject.category;

		if (typeof category === "object" && category !== null) {
			const categoryType = (category as Record<string, unknown>).type;

			if (typeof categoryType === "string") {
				const source =
					typeof subject.source === "string" ? `${subject.source} · ` : "";

				return `${source}${categoryType.replaceAll("_", " ")}`;
			}
		}
	}

	if (subject.type === "confidence" && typeof subject.confidence === "number") {
		return `confidence ≥ ${subject.confidence}`;
	}

	const parts = [subject.source, subject.type].filter(
		(part) => typeof part === "string" && part !== "",
	);

	return parts.join(" · ") || "subject";
};

const conditionLabel = (condition: ConditionWire): string => {
	const left = subjectLabel(condition.left?.subject);

	switch (condition.type) {
		case "is_true":
			return left;
		case "is_false":
			return `¬ ${left}`;
		case "is_greater_than_or_equal": {
			const right = subjectLabel(condition.right?.subject);

			return `${left} ≥ ${right}`;
		}
		default:
			return condition.type ? `${condition.type} · ${left}` : left;
	}
};

const branchSummary = (branch: PlaybookBranch): string => {
	const conditions = branch.condition_group?.conditions ?? [];

	if (conditions.length === 0) {
		return "branch";
	}

	return conditions
		.map(conditionLabel)
		.join(branch.condition_group?.boolean === "or" ? " OR " : " AND ");
};

const actionLabel = (action: PlaybookBranch["action"]): string | null => {
	if (!action?.type) {
		return null;
	}

	const side = action.side ? ` ${action.side}` : "";
	const fraction =
		action.fraction && action.fraction > 0 && action.fraction < 1
			? ` ${Math.round(action.fraction * 100)}%`
			: "";

	return `${action.type}${side}${fraction}`;
};

type NodeState = "matched" | "action" | "rejected" | "parked" | "idle";

const nodeStateFor = (step: WalkStep | null, active: boolean): NodeState => {
	if (active) {
		return "parked";
	}

	switch (step?.outcome) {
		case "matched":
			return "matched";
		case "action":
			return "action";
		case "rejected":
			return "rejected";
		case "parked":
			return "parked";
		default:
			return "idle";
	}
};

const NODE_TONE: Record<NodeState, string> = {
	matched: "border-emerald-500/60 bg-emerald-500/12 text-emerald-50",
	action: "border-sky-400/70 bg-sky-400/18 text-sky-50 shadow-sky-400/30",
	rejected: "border-border/70 bg-card/40 text-muted-foreground/80",
	parked: "border-amber-500/60 bg-amber-500/12 text-amber-50",
	idle: "border-border bg-card/50 text-foreground/70",
};

const EDGE_TONE: Record<NodeState, string> = {
	matched: "stroke-emerald-400/70",
	action: "stroke-sky-400/80",
	rejected: "stroke-border/50",
	parked: "stroke-amber-400/70",
	idle: "stroke-border/40",
};

/*
A flattened node ready to render: carries its own state plus geometry so the
SVG edge layer and the DOM node layer agree on positions. The tree is laid out
in columns by depth (left → right) so the walk reads as flow.
*/
type FlatNode = {
	id: string;
	branch: PlaybookBranch;
	depth: number;
	path: number[];
	parentId: string | null;
	state: NodeState;
	row: number;
};

const flatten = (
	branches: PlaybookBranch[],
	walkTrace: WalkTrace | null,
): { nodes: FlatNode[]; maxDepth: number; rows: number } => {
	const nodes: FlatNode[] = [];
	let rowCursor = 0;
	let maxDepth = 0;

	const visit = (
		branch: PlaybookBranch,
		depth: number,
		path: number[],
		parentId: string | null,
	): number => {
		const id = path.join(".");
		const step = walkStepForPath(walkTrace, path);
		const active = pathIsActive(walkTrace, path);
		const children = branch.branches ?? [];

		maxDepth = Math.max(maxDepth, depth);

		const node: FlatNode = {
			id,
			branch,
			depth,
			path,
			parentId,
			state: nodeStateFor(step, active),
			row: 0,
		};
		nodes.push(node);

		if (children.length === 0) {
			node.row = rowCursor;
			rowCursor += 1;
			return node.row;
		}

		const childRows = children.map((child, index) =>
			visit(child, depth + 1, [...path, index], id),
		);

		// Parent sits at the average row of its children → tidy vertical centering.
		node.row =
			childRows.reduce((sum, value) => sum + value, 0) / childRows.length;

		return node.row;
	};

	branches.forEach((branch, index) => {
		visit(branch, 0, [index], null);
	});

	return { nodes, maxDepth, rows: rowCursor };
};

const COL_WIDTH = 240;
const ROW_HEIGHT = 64;
const NODE_WIDTH = 200;
const NODE_HEIGHT = 48;

const DecisionGraph = ({
	branches,
	walkTrace,
}: {
	branches: PlaybookBranch[];
	walkTrace: WalkTrace | null;
}) => {
	const { nodes, maxDepth, rows } = flatten(branches, walkTrace);
	const byId = new Map(nodes.map((node) => [node.id, node]));

	const width = (maxDepth + 1) * COL_WIDTH;
	const height = Math.max(rows, 1) * ROW_HEIGHT;

	const xOf = (node: FlatNode) => node.depth * COL_WIDTH;
	const yOf = (node: FlatNode) => node.row * ROW_HEIGHT;

	return (
		<div className="relative h-full w-full overflow-auto">
			<div className="relative" style={{ width, height }}>
				<svg
					className="pointer-events-none absolute inset-0"
					width={width}
					height={height}
					aria-hidden="true"
				>
					{nodes.map((node) => {
						if (node.parentId === null) {
							return null;
						}

						const parent = byId.get(node.parentId);

						if (parent === undefined) {
							return null;
						}

						const x1 = xOf(parent) + NODE_WIDTH;
						const y1 = yOf(parent) + NODE_HEIGHT / 2;
						const x2 = xOf(node);
						const y2 = yOf(node) + NODE_HEIGHT / 2;
						const mid = (x1 + x2) / 2;

						return (
							<path
								key={`edge-${node.id}`}
								d={`M ${x1} ${y1} C ${mid} ${y1}, ${mid} ${y2}, ${x2} ${y2}`}
								fill="none"
								strokeWidth={node.state === "idle" ? 1 : 2}
								className={cn(EDGE_TONE[node.state], "transition-colors")}
							/>
						);
					})}
				</svg>

				{nodes.map((node) => {
					const action = actionLabel(node.branch.action);

					return (
						<div
							key={`node-${node.id}`}
							className={cn(
								"absolute flex flex-col justify-center rounded-lg border px-3 transition-colors",
								node.state === "action" ? "shadow-lg" : "",
								NODE_TONE[node.state],
							)}
							style={{
								left: xOf(node),
								top: yOf(node),
								width: NODE_WIDTH,
								height: NODE_HEIGHT,
							}}
						>
							<p className="truncate font-mono text-[10px] leading-tight">
								{branchSummary(node.branch)}
							</p>
							{action ? (
								<p className="truncate text-[9px] uppercase tracking-wide text-sky-300">
									{action}
								</p>
							) : null}
						</div>
					);
				})}
			</div>
		</div>
	);
};

const WalkLegend = () => (
	<div className="flex flex-wrap gap-3 text-[10px] text-muted-foreground">
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-emerald-400" />
			matched
		</span>
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-amber-400" />
			parked
		</span>
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-sky-400" />
			action
		</span>
		<span className="inline-flex items-center gap-1.5">
			<span className="size-2 rounded-full bg-muted-foreground/50" />
			rejected
		</span>
	</div>
);

/*
The latest-walk store updates ~2000×/sec. Subscribing the graph directly would
thrash the DOM. We snapshot the store on an animation frame instead, so the
graph repaints at most once per frame (~60fps) regardless of feed rate.
*/
const useCoalescedWalk = (): WalkTrace | null => {
	const [walk, setWalk] = useState<WalkTrace | null>(
		() => playbookStore.state.walkTrace,
	);
	const frame = useRef<number | null>(null);
	const pending = useRef<WalkTrace | null>(playbookStore.state.walkTrace);

	useEffect(() => {
		const subscription = playbookStore.subscribe(() => {
			pending.current = playbookStore.state.walkTrace;

			if (frame.current !== null) {
				return;
			}

			frame.current = requestAnimationFrame(() => {
				frame.current = null;
				setWalk(pending.current);
			});
		});

		return () => {
			subscription.unsubscribe();

			if (frame.current !== null) {
				cancelAnimationFrame(frame.current);
			}
		};
	}, []);

	return walk;
};

const DecisionsPage = () => {
	const branches = useStore(playbookStore, (state) => state.branches);
	const walkTrace = useCoalescedWalk();

	return (
		<div className="flex h-full min-h-0 w-full flex-col gap-3">
			<div className="flex shrink-0 items-center justify-between gap-4">
				<div className="flex flex-col gap-1">
					<h1 className="text-lg font-semibold">Decision Tree</h1>
					<p className="text-xs text-muted-foreground">
						Live playbook walk — flowing left to right. Nodes light as each
						symbol descends, gates, or fires an action.
					</p>
				</div>
				<div className="flex flex-col items-end gap-1">
					{walkTrace ? (
						<p className="font-mono text-xs tabular-nums">
							{walkTrace.symbol}
						</p>
					) : null}
					<WalkLegend />
				</div>
			</div>

			<div className="min-h-0 flex-1 rounded-xl border border-border bg-card/40 p-2">
				{branches.length === 0 ? (
					<p className="p-4 text-sm text-muted-foreground">
						Waiting for the story to publish the playbook…
					</p>
				) : (
					<DecisionGraph branches={branches} walkTrace={walkTrace} />
				)}
			</div>
		</div>
	);
};

export const Route = createFileRoute("/decisions")({
	component: DecisionsPage,
});
