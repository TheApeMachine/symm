import { createStore } from "@tanstack/react-store";
import type { StrategyDecision } from "#/types/thesis";

const asDecisions = (frame: unknown): StrategyDecision[] => {
	if (!Array.isArray(frame)) {
		return [];
	}

	return frame.filter(
		(row): row is StrategyDecision =>
			typeof row === "object" &&
			row !== null &&
			typeof (row as StrategyDecision).symbol === "string" &&
			typeof (row as StrategyDecision).action === "string",
	);
};

/*
decisionStore retains the backend strategy decision snapshot published with each
thesis tick so the decision surface can show utility and alternatives beside
gate verdicts without conflating the two decision layers.
*/
export const decisionStore = createStore(
	{
		decisions: [] as StrategyDecision[],
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState(() => ({
				decisions: asDecisions(frame),
			})),
		reset: () =>
			setState(() => ({
				decisions: [],
			})),
	}),
);
