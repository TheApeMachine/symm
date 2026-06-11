import { createFileRoute } from "@tanstack/react-router";
import { useState } from "react";
import { useWebSocket } from "react-use-websocket/dist/lib/use-websocket";
import {
	applyDecisionTreeStats,
	applyGlobalFrame,
	statusSocketHandlers,
} from "#/providers/global-frames";

/*
Decision Tree — the embedded playbook from tree.yml, published by story as-is.
*/

type SubjectWire = {
	source?: string;
	type?: string;
	category?: { type?: string };
	holding?: { held?: boolean };
	confidence?: number;
};

type ConditionWire = {
	type?: string;
	left?: { subject?: SubjectWire };
	right?: { subject?: SubjectWire };
};

type ConditionGroupWire = {
	boolean?: string;
	conditions?: ConditionWire[];
};

type ActionWire = {
	type?: string;
	side?: string;
	fraction?: number;
};

type BranchWire = {
	condition_group?: ConditionGroupWire;
	action?: ActionWire;
	branches?: BranchWire[];
};

type PlaybookFrame = {
	chart: "decision_tree";
	branches: BranchWire[];
};

const socketUrl =
	(import.meta.env.VITE_SYMM_WS_URL as string | undefined)?.trim() ||
	"ws://127.0.0.1:8765/ws";

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null;

const isBranchWire = (value: unknown): value is BranchWire => {
	if (!isRecord(value)) {
		return false;
	}

	if (value.condition_group !== undefined && !isRecord(value.condition_group)) {
		return false;
	}

	if (value.action !== undefined && !isRecord(value.action)) {
		return false;
	}

	if (value.branches !== undefined) {
		if (!Array.isArray(value.branches)) {
			return false;
		}

		return value.branches.every(isBranchWire);
	}

	return true;
};

const isPlaybookFrame = (value: unknown): value is PlaybookFrame => {
	if (!isRecord(value) || value.chart !== "decision_tree") {
		return false;
	}

	if (!Array.isArray(value.branches)) {
		return false;
	}

	return value.branches.every(isBranchWire);
};

const subjectLabel = (subject: SubjectWire | undefined): string => {
	if (!subject) {
		return "subject";
	}

	if (subject.type === "holding") {
		return subject.holding?.held ? "holding" : "flat";
	}

	if (subject.type === "category" && subject.category?.type) {
		const source = subject.source ? `${subject.source} · ` : "";

		return `${source}${subject.category.type.replaceAll("_", " ")}`;
	}

	if (subject.type === "confidence" && subject.confidence !== undefined) {
		return `confidence ≥ ${subject.confidence}`;
	}

	const parts = [subject.source, subject.type].filter(Boolean);

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

const branchSummary = (branch: BranchWire): string => {
	const conditions = branch.condition_group?.conditions ?? [];

	if (conditions.length === 0) {
		return "branch";
	}

	return conditions
		.map(conditionLabel)
		.join(branch.condition_group?.boolean === "or" ? " OR " : " AND ");
};

const actionLabel = (action: ActionWire | undefined): string | null => {
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

const BranchNode = ({
	branch,
	depth,
}: {
	branch: BranchWire;
	depth: number;
}) => {
	const action = actionLabel(branch.action);
	const children = branch.branches ?? [];

	return (
		<div
			className="flex flex-col gap-2 border-l border-border pl-3"
			style={{ marginLeft: depth > 0 ? 8 : 0 }}
		>
			<div className="rounded-lg border border-border bg-card/60 px-3 py-2">
				<p className="font-mono text-[11px] leading-snug text-foreground">
					{branchSummary(branch)}
				</p>
				{action ? (
					<p className="mt-1 text-[10px] uppercase tracking-wide text-sky-300">
						{action}
					</p>
				) : null}
			</div>

			{children.length > 0 ? (
				<div className="flex flex-col gap-2">
					{children.map((child, index) => (
						<BranchNode
							key={`${depth}-${index}-${branchSummary(child)}`}
							branch={child}
							depth={depth + 1}
						/>
					))}
				</div>
			) : null}
		</div>
	);
};

const DecisionsPage = () => {
	const [frame, setFrame] = useState<PlaybookFrame | null>(null);

	useWebSocket(socketUrl, {
		...statusSocketHandlers,
		onMessage: (event) => {
			try {
				const raw = JSON.parse(event.data) as Record<string, unknown>;

				applyDecisionTreeStats(raw);

				if (applyGlobalFrame(raw)) {
					return;
				}

				if (isPlaybookFrame(raw)) {
					setFrame(raw);
				}
			} catch (error) {
				console.error(
					"decision_tree frame parse/validation failed",
					error,
					event.data,
				);
			}
		},
	});

	const branches = frame?.branches ?? [];

	return (
		<div className="flex h-full min-h-0 w-full flex-col gap-3">
			<div className="flex flex-col">
				<h1 className="text-lg font-semibold">Decision Tree</h1>
				<p className="text-xs text-muted-foreground">
					The embedded playbook from tree.yml.
				</p>
			</div>

			<div className="min-h-0 flex-1 overflow-auto rounded-xl border border-border bg-card/40 p-4">
				{branches.length === 0 ? (
					<p className="text-sm text-muted-foreground">
						Waiting for the story to publish the playbook…
					</p>
				) : (
					<div className="flex flex-col gap-4">
						{branches.map((branch, index) => (
							<BranchNode
								key={`root-${index}-${branchSummary(branch)}`}
								branch={branch}
								depth={0}
							/>
						))}
					</div>
				)}
			</div>
		</div>
	);
};

export const Route = createFileRoute("/decisions")({
	component: DecisionsPage,
});
