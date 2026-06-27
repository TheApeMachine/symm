import { createStore } from "@tanstack/react-store";

type DecisionFrame = Record<string, unknown>;

type DecisionsState = {
	frame: DecisionFrame | null;
	bySymbol: Record<string, DecisionFrame>;
};

const decisionSymbol = (decision: DecisionFrame): string =>
	typeof decision.symbol === "string" ? decision.symbol : "";

const decisionsFromSymbols = (
	bySymbol: Record<string, DecisionFrame>,
): DecisionFrame[] =>
	Object.values(bySymbol).sort((left, right) => {
		const leftScore =
			typeof left.score === "number"
				? left.score
				: typeof left.confidence === "number"
					? left.confidence
					: 0;
		const rightScore =
			typeof right.score === "number"
				? right.score
				: typeof right.confidence === "number"
					? right.confidence
					: 0;

		return rightScore - leftScore;
	});

const aggregateFrame = (
	source: DecisionFrame,
	bySymbol: Record<string, DecisionFrame>,
): DecisionFrame => ({
	role: "decisions",
	seq: source.seq,
	observed_at: source.observed_at,
	decisions: decisionsFromSymbols(bySymbol),
});

/*
decisionsStore collects the backend's role=decision artifacts. The trader emits
ONE artifact per candidate (symbol, side, type, price, quantity, confidence,
verdict, why, observed_at, seq) and has already decided — the frontend renders
verdict/why/confidence directly, never re-scoring. Artifacts are keyed by symbol
so the latest decision per symbol replaces the prior one.
*/
export const decisionsStore = createStore(
	{ frame: null, bySymbol: {} } as DecisionsState,
	({ setState }) => ({
		updateFrame: (frame: Record<string, unknown>) =>
			setState((prev) => {
				if (Array.isArray(frame.decisions)) {
					const bySymbol: Record<string, DecisionFrame> = {};

					for (const decision of frame.decisions) {
						if (
							typeof decision !== "object" ||
							decision === null ||
							Array.isArray(decision)
						) {
							continue;
						}

						const row = decision as DecisionFrame;
						const symbol = decisionSymbol(row);

						if (symbol === "") {
							continue;
						}

						bySymbol[symbol] = row;
					}

					return { frame, bySymbol };
				}

				const symbol = decisionSymbol(frame);

				if (symbol === "") {
					return prev;
				}

				const bySymbol = { ...prev.bySymbol, [symbol]: frame };

				return {
					bySymbol,
					frame: aggregateFrame(frame, bySymbol),
				};
			}),
		reset: () => setState(() => ({ frame: null, bySymbol: {} })),
	}),
);
