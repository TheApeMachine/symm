import { DecisionStage } from "#/components/terminal/decision-chain-stages";
import { readDecisionTrace } from "#/components/terminal/decision-trace-model";
import {
	CouncilPanel,
	DecisionTree,
} from "#/components/terminal/decision-tree";
import type { Decision } from "#/providers/telemetry/telemetry/decision";

/*
DecisionMCTSStage shows the live reasoning behind one decision: the War Room's
deliberation, then the causal search tree it drove.
*/
export const DecisionMCTSStage = ({ decision }: { decision: Decision }) => {
	const trace = readDecisionTrace(decision);

	// The stage numbering stays fixed whether or not a search ran: an operator
	// scanning the chain should find the same reasoning stage in the same
	// place every frame, rather than watching the columns renumber themselves
	// as data arrives.
	if (!trace) {
		return (
			<>
				<DecisionStage title="3 · war room" meta="advisor deliberation">
					<span className="text-(--f4) text-[9px]">
						no advisor has spoken for this symbol
					</span>
				</DecisionStage>
				<DecisionStage title="4 · causal search" meta="no search this round">
					<span className="text-(--f4) text-[9px]">
						the council did not deliberate, or no transition model was available
					</span>
				</DecisionStage>
			</>
		);
	}

	return (
		<>
			<DecisionStage title="3 · war room" meta="advisor deliberation">
				<CouncilPanel trace={trace} />
			</DecisionStage>
			<DecisionStage
				title="4 · causal search"
				meta="mcts + pearl counterfactuals"
			>
				<DecisionTree trace={trace} />
			</DecisionStage>
		</>
	);
};
