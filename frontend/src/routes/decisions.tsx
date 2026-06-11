import { createFileRoute } from "@tanstack/react-router";
import { useSelector } from "@tanstack/react-store";
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

const outcomeLabel = (
	step: WalkStep | null,
	active: boolean,
): string | null => {
	if (active) {
		return "parked — waiting for next timeline tick";
	}

	switch (step?.outcome) {
		case "matched":
			return "matched — descending into children";
		case "action":
			return step.reason ? `action — ${step.reason}` : "action selected";
		case "rejected":
			return step.reason ? `rejected — ${step.reason}` : "rejected";
		case "parked":
			return step.reason ?? "parked — waiting for timeline";
		default:
			return null;
	}
};

const branchCardClass = (step: WalkStep | null, active: boolean): string => {
	if (active || step?.outcome === "parked") {
		return "border-amber-500/60 bg-amber-500/10";
	}

	switch (step?.outcome) {
		case "matched":
			return "border-emerald-500/50 bg-emerald-500/10";
		case "action":
			return "border-sky-400/70 bg-sky-400/15";
		case "rejected":
			return "border-border/80 bg-card/30 opacity-80";
		default:
			return "border-border bg-card/60";
	}
};

const BranchNode = ({
	branch,
	depth,
	path,
	walkTrace,
}: {
	branch: PlaybookBranch;
	depth: number;
	path: number[];
	walkTrace: WalkTrace | null;
}) => {
	const action = actionLabel(branch.action);
	const children = branch.branches ?? [];
	const step = walkStepForPath(walkTrace, path);
	const active = pathIsActive(walkTrace, path);
	const status = outcomeLabel(step, active);

	return (
		<div
			className="flex flex-col gap-2 border-l border-border pl-3"
			style={{ marginLeft: depth > 0 ? 8 : 0 }}
		>
			<div
				className={cn(
					"rounded-lg border px-3 py-2 transition-colors",
					branchCardClass(step, active),
				)}
			>
				<p className="font-mono text-[11px] leading-snug text-foreground">
					{branchSummary(branch)}
				</p>
				{action ? (
					<p className="mt-1 text-[10px] uppercase tracking-wide text-sky-300">
						{action}
					</p>
				) : null}
				{status ? (
					<p className="mt-2 text-[10px] leading-snug text-muted-foreground">
						{status}
					</p>
				) : null}
			</div>

			{children.length > 0 ? (
				<div className="flex flex-col gap-2">
					{children.map((child, index) => (
						<BranchNode
							key={`${path.join("-")}-${index}-${branchSummary(child)}`}
							branch={child}
							depth={depth + 1}
							path={[...path, index]}
							walkTrace={walkTrace}
						/>
					))}
				</div>
			) : null}
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
			rejected (reason shown)
		</span>
	</div>
);

const DecisionsPage = () => {
	const branches = useSelector(playbookStore, (state) => state.branches);
	const walkTrace = useSelector(playbookStore, (state) => state.walkTrace);

	return (
		<div className="flex h-full min-h-0 w-full flex-col gap-3">
			<div className="flex flex-col gap-2">
				<h1 className="text-lg font-semibold">Decision Tree</h1>
				<p className="text-xs text-muted-foreground">
					Live playbook walk — branches light up as the story evaluates each
					symbol spectrum.
				</p>
				{walkTrace ? (
					<div className="flex flex-col gap-1 rounded-lg border border-border bg-card/40 px-3 py-2">
						<p className="text-xs font-medium tabular-nums">
							Latest walk · {walkTrace.symbol}
						</p>
						<WalkLegend />
					</div>
				) : (
					<p className="text-xs text-muted-foreground">
						Waiting for the first playbook evaluation…
					</p>
				)}
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
								path={[index]}
								walkTrace={walkTrace}
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
