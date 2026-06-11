import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
import { type PlaybookBranch, playbookStore } from "#/collections/playbook";

/*
Decision Tree — the embedded playbook from tree.yml, published by story as-is.
*/

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

const BranchNode = ({
	branch,
	depth,
}: {
	branch: PlaybookBranch;
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
	const branches = useSelector(playbookStore, (state) => state.branches);

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
