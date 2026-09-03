import type {
	DecisionTraceModel,
	TraceNode,
} from "#/components/terminal/decision-trace-model";

const fmt = (value: number, digits = 4): string =>
	Number.isFinite(value) ? value.toFixed(digits) : "—";

const pct = (value: number, digits = 1): string =>
	Number.isFinite(value) ? `${(value * 100).toFixed(digits)}%` : "—";

/*
evidenceSplit is how much of a node's confidence came from real rollouts versus
Pearl's counterfactuals. It is the single most useful thing to see live: a
branch carried mostly by virtual evidence is a branch the search never actually
walked, and an operator should weigh it accordingly.
*/
const evidenceSplit = (node: TraceNode): number => {
	const total = node.visits + node.counterfactualMass;

	return total <= 0 ? 0 : node.visits / total;
};

const NodeRow = ({
	node,
	maxVisits,
	path,
}: {
	node: TraceNode;
	maxVisits: number;
	path: string;
}) => {
	const real = evidenceSplit(node);
	const width = maxVisits <= 0 ? 0 : (node.effectiveVisits / maxVisits) * 100;

	return (
		<div
			className="flex flex-col gap-0.5 border-(--line) border-l pl-2"
			style={{ marginLeft: node.depth === 0 ? 0 : 8 }}
			data-selected={node.selected ? "true" : "false"}
			data-pruned={node.pruned ? "true" : "false"}
		>
			<div className="flex items-center justify-between gap-2">
				<span
					className={
						node.pruned
							? "text-(--f4) line-through"
							: node.selected
								? "font-semibold text-(--acc)"
								: "text-(--f2)"
					}
				>
					{node.action}
					{node.pruned ? " · pruned" : ""}
				</span>
				<span className="shrink-0 text-[8px] text-(--f4)">
					<b className="font-normal text-(--f3)">{node.visits}</b>
					{node.counterfactualMass > 0.005 ? (
						<>
							{" + "}
							<b className="font-normal text-(--acc)">
								{node.counterfactualMass.toFixed(2)}
							</b>
							{" cf"}
						</>
					) : null}
				</span>
			</div>

			{/* Visit bar, split by how much of the evidence was actually walked. */}
			<div className="h-0.5 w-full overflow-hidden rounded-full bg-(--sunken)">
				<div
					className="flex h-full"
					style={{ width: `${Math.min(width, 100)}%` }}
				>
					<div
						className="h-full bg-(--f3)"
						style={{ width: `${real * 100}%` }}
					/>
					<div
						className="h-full bg-(--acc)"
						style={{ width: `${(1 - real) * 100}%` }}
					/>
				</div>
			</div>

			<div className="flex items-center justify-between gap-2 text-[8px] text-(--f4)">
				<span>
					blended{" "}
					<b className="font-normal text-(--f2)">{fmt(node.blendedValue)}</b>
				</span>
				{node.causalExpectationDefined ? (
					<span title="E[reward | do(action)]">
						do{" "}
						<b className="font-normal text-(--f3)">
							{fmt(node.causalExpectation, 3)}
						</b>
					</span>
				) : (
					<span className="text-(--f5)">unidentified</span>
				)}
			</div>

			{node.children.length > 0 ? (
				<div className="mt-0.5 flex flex-col gap-1">
					{node.children.map((child, index) => {
						// The path is the branch's position in the tree, which is
						// stable across frames as the search deepens; a bare index
						// would remount rows whenever sibling order shifted.
						const childPath = `${path}/${child.action}:${index}`;

						return (
							<NodeRow
								key={childPath}
								node={child}
								maxVisits={maxVisits}
								path={childPath}
							/>
						);
					})}
				</div>
			) : null}
		</div>
	);
};

const peakVisits = (node: TraceNode): number => {
	let peak = node.effectiveVisits;

	for (const child of node.children) {
		const childPeak = peakVisits(child);

		if (childPeak > peak) {
			peak = childPeak;
		}
	}

	return peak;
};

/*
DecisionTree renders the live MCTS + Pearl search tree for one decision.
*/
export const DecisionTree = ({ trace }: { trace: DecisionTraceModel }) => {
	if (!trace.tree) {
		return (
			<div className="px-2 py-3 text-[9px] text-(--f4)">
				no search tree — {trace.identificationStatus || "not run"}
			</div>
		);
	}

	const maxVisits = peakVisits(trace.tree);

	return (
		<div className="flex flex-col gap-1.5">
			<div className="flex items-center justify-between gap-2 text-[8px] text-(--f4)">
				<span>
					<b className="font-normal text-(--f2)">{trace.iterations}</b> rollouts
					· <b className="font-normal text-(--f2)">{trace.horizon}</b> steps
				</span>
				<span className="truncate">{trace.transitionSource}</span>
			</div>

			<div className="flex flex-col gap-1">
				<NodeRow node={trace.tree} maxVisits={maxVisits} path="root" />
			</div>

			<div className="flex items-center justify-between gap-2 border-(--line) border-t pt-1 text-[8px]">
				<span className="text-(--f4)">
					expected{" "}
					<b className="font-normal text-(--f2)">
						{fmt(trace.expectedOutcome)}
					</b>
					{" ± "}
					<b className="font-normal text-(--f3)">
						{fmt(trace.outcomeUncertainty, 3)}
					</b>
				</span>
				<span
					className={
						trace.decisionUnavailable
							? "font-semibold text-(--down)"
							: "font-semibold text-(--acc)"
					}
				>
					{trace.decisionUnavailable ? "unavailable" : trace.recommendedAction}
				</span>
			</div>

			{/* Legend: the real-vs-virtual split is the whole point of the bar. */}
			<div className="flex items-center gap-2 text-[7px] text-(--f5)">
				<span className="flex items-center gap-1">
					<i className="inline-block h-1 w-2 bg-(--f3)" /> rollouts
				</span>
				<span className="flex items-center gap-1">
					<i className="inline-block h-1 w-2 bg-(--acc)" /> counterfactual
				</span>
			</div>
		</div>
	);
};

/*
CouncilPanel shows the War Room deliberation that produced the odds the search
then used, including any veto or synergy that fired.
*/
export const CouncilPanel = ({ trace }: { trace: DecisionTraceModel }) => (
	<div className="flex flex-col gap-1">
		<div className="flex items-center justify-between gap-2">
			<span className="text-(--f4) text-[8px]">
				<b className="font-normal text-(--f2)">{trace.council.participants}</b>{" "}
				advisors
			</span>
			<span className="font-semibold text-(--f2)">
				{trace.council.dominantMove || "—"}{" "}
				<span className="font-normal text-(--f4)">
					{pct(trace.council.confidence)}
				</span>
			</span>
		</div>

		{trace.council.synergies.map((reason) => (
			<div key={reason} className="text-[8px] text-(--up) leading-tight">
				+ {reason}
			</div>
		))}

		{trace.council.vetoes.map((reason) => (
			<div key={reason} className="text-[8px] text-(--down) leading-tight">
				− {reason}
			</div>
		))}
	</div>
);
