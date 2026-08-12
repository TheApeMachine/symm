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
	className="flex-1 rounded-xs border border-(--line) bg-(--sunken) px-1.5 py-1"
>
		<span
			data-paint={`trace.mcts.branches.${index}.action`}
			className="block truncate text-(--f4)"
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
	<DecisionStage title="3 · graph search" meta="root branches explored">
		<div className="flex items-stretch gap-2">
			<BranchBox index={0} />
			<BranchBox index={1} />
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
				className="max-w-30 truncate font-semibold text-(--acc)"
			/>
		</div>
	</DecisionStage>
);
