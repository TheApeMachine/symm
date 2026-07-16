import { createStore } from "@tanstack/react-store";
import type { ThesisHypothesis } from "#/types/thesis";
import { Circular, type CircularBuffer } from "./circular";

const HYPOTHESIS_HISTORY_LIMIT = 50;

const hypothesisKey = (row: ThesisHypothesis): string =>
	JSON.stringify([row.symbol, row.source, row.claim]);

const asHypotheses = (frame: unknown): ThesisHypothesis[] => {
	if (!Array.isArray(frame)) {
		return [];
	}

	return frame.filter((row): row is ThesisHypothesis => {
		if (typeof row !== "object" || row === null) {
			return false;
		}

		const hypothesis = row as ThesisHypothesis;

		return (
			typeof hypothesis.symbol === "string" &&
			hypothesis.symbol.length > 0 &&
			typeof hypothesis.source === "string" &&
			hypothesis.source.length > 0 &&
			typeof hypothesis.claim === "string" &&
			hypothesis.claim.length > 0
		);
	});
};

/*
hypothesisValues returns the newest retained hypothesis row for each identity.
*/
export const hypothesisValues = (
	hypotheses: Record<string, CircularBuffer<ThesisHypothesis>>,
): ThesisHypothesis[] =>
	Object.keys(hypotheses)
		.sort()
		.flatMap((key) => {
			const hypothesis = hypotheses[key]?.values().at(-1);

			return hypothesis === undefined ? [] : [hypothesis];
		});

/*
hypothesesStore retains backend thesis hypotheses in bounded circular buffers
so partial tick frames cannot erase retained symbol evidence.
*/
export const hypothesesStore = createStore(
	{
		hypotheses: {} as Record<string, CircularBuffer<ThesisHypothesis>>,
		version: 0,
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => {
				const rows = asHypotheses(frame);

				if (rows.length === 0) {
					return prev;
				}

				const hypotheses = prev.hypotheses;

				for (const row of rows) {
					const key = hypothesisKey(row);

					if (!hypotheses[key]) {
						hypotheses[key] = Circular<ThesisHypothesis>(
							HYPOTHESIS_HISTORY_LIMIT,
						);
					}

					hypotheses[key].push(row);
				}

				return {
					hypotheses,
					version: prev.version + 1,
				};
			}),
		reset: () =>
			setState(() => ({
				hypotheses: {},
				version: 0,
			})),
	}),
);
