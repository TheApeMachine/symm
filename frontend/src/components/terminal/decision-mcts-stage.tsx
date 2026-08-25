import type { Decision } from "#/types/thesis";
import { DecisionStage } from "#/components/terminal/decision-chain-stages";

const fmtn = (value: number | undefined, digits: number): string =>
	value === undefined ? "" : value.toFixed(digits);

const BranchBox = ({ decision, index }: { decision: Decision; index: 0 | 1 }) => {
	const branch = decision.trace?.mcts?.branches?.[index];

	return (
		<div className="flex-1 rounded-xs border border-(--line) bg-(--sunken) px-1.5 py-1">
			<span className="block truncate text-(--f4)">{branch?.action ?? ""}</span>
			<div className="flex items-baseline justify-between gap-2">
				<b className="font-normal text-(--f2)">{fmtn(branch?.meanReward, 5)}</b>
				<span className="text-[7px] text-(--f4)">
					<b className="font-normal text-(--f3)">{branch?.visits ?? ""}</b> visits
				</span>
			</div>
		</div>
	);
};

export const DecisionMCTSStage = ({ decision }: { decision: Decision }) => (
	<DecisionStage title="3 · graph search" meta="root branches explored">
		<div className="flex items-stretch gap-2">
			<BranchBox decision={decision} index={0} />
			<BranchBox decision={decision} index={1} />
		</div>
		<div className="flex items-center justify-between gap-2">
			<span className="text-(--f4)">
				<b className="font-normal text-(--f2)">{decision.trace?.mcts?.iterations ?? ""}</b> simulations
			</span>
			<span className="max-w-30 truncate font-semibold text-(--acc)">
				{decision.trace?.mcts?.recommendedAction ?? ""}
			</span>
		</div>
	</DecisionStage>
);
