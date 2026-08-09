import { DecisionStage } from "#/components/terminal/decision-chain-stages";

/*
DecisionMCTSStage shows the root branches the causal search actually explored:
each child action it tried, how many simulations visited it, and the mean
reward those simulations produced. The recommended action is the branch with
the most visits — the search's own robust-child rule, not a value comparison
recomputed here. Two slots are rendered because GetPossibleActions caps the
root at two children today (stand aside, enter); an absent second branch
(single-action states) just reads blank rather than collapsing, since
data-paint-absent cannot be combined with data-paint-prop on an element that
still needs its nested children (the engine writes textContent unconditionally
on the absent path, which would wipe them).
*/
const BranchBox = ({ index }: { index: 0 | 1 }) => (
	<div
		data-paint={`trace.mcts.branches.${index}.action`}
		data-paint-prop="dataset.action"
		className="flex-1 rounded-xs border border-(--line) bg-(--sunken) px-1.5 py-1 data-[action=nothing]:border-(--up) data-[action=nothing]:bg-[color-mix(in_srgb,var(--up)_8%,transparent)]"
	>
		<span
			data-paint={`trace.mcts.branches.${index}.action`}
			className="text-(--f4) uppercase"
		/>
		<div className="flex items-baseline justify-between gap-2">
			<b
				data-paint={`trace.mcts.branches.${index}.meanReward`}
				data-paint-format=".5f"
				className="font-normal text-(--f2)"
			/>
			<span className="text-[7px] text-(--f4)">
				<b
					data-paint={`trace.mcts.branches.${index}.visits`}
					className="font-normal text-(--f3)"
				/>{" "}
				visits
			</span>
		</div>
	</div>
);

export const DecisionMCTSStage = () => (
	<DecisionStage title="3 · causal search" meta="root branches explored">
		<div className="flex items-stretch gap-2">
			<BranchBox index={0} />
			<BranchBox index={1} />
		</div>
		<div className="flex justify-between gap-2 text-(--f4)">
			<span>
				treatment{" "}
				<b
					data-paint="trace.mcts.treatment"
					data-paint-format=".3%"
					className="font-normal text-(--f2)"
				/>
			</span>
			<span>
				precision{" "}
				<b
					data-paint="trace.mcts.precision"
					data-paint-format=".3f"
					className="font-normal text-(--f2)"
				/>
			</span>
		</div>
		<div className="flex items-center justify-between gap-2">
			<span className="text-(--f4)">
				<b
					data-paint="trace.mcts.iterations"
					className="font-normal text-(--f2)"
				/>{" "}
				simulations
			</span>
			<span
				data-paint="trace.mcts.recommendedAction"
				className="font-semibold text-(--acc) uppercase"
			/>
		</div>
	</DecisionStage>
);
