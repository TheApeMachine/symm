import { useSelector } from "@tanstack/react-store";
import { signals } from "#/collections/app";
import { terminalStore } from "#/collections/terminal";
import { selectDecisions } from "#/components/dashboard/decisions";
import {
	setDecisionsPendingFocus,
	setDecisionsScopeSymbol,
} from "#/components/terminal/decision-side";
import { DecisionList, type DecisionRow } from "./decision-list";

export type DecisionsProps = {
	decisions?: DecisionRow[] | any;
	className?: string;
};

const inspectDecision = (symbol: string) => {
	setDecisionsScopeSymbol(symbol);
	setDecisionsPendingFocus(symbol);
	terminalStore.actions.openThesis(symbol);
};

export const Decisions = ({
	decisions: propDecisions,
	className,
}: DecisionsProps = {}) => {
	const streamDecisions = useSelector(signals.strategy, selectDecisions);
	const decisions =
		propDecisions !== undefined
			? Array.isArray(propDecisions)
				? propDecisions
				: propDecisions?.toArray
					? propDecisions.toArray()
					: []
			: streamDecisions;

	return (
		<DecisionList
			decisions={decisions}
			onInspect={inspectDecision}
			className={className}
		/>
	);
};
