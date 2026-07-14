import { createStore } from "@tanstack/react-store";
import type { Finding } from "#/types/thesis";

const asFindings = (frame: unknown): Finding[] => {
	if (!Array.isArray(frame)) {
		return [];
	}

	return frame.filter(
		(row): row is Finding =>
			typeof row === "object" &&
			row !== null &&
			typeof (row as Finding).component === "string" &&
			typeof (row as Finding).condition === "string",
	);
};

/*
findingsStore retains the backend thesis.Findings snapshot so PostMortem
evidence can be inspected without mutating or replaying the live model state.
*/
export const findingsStore = createStore(
	{
		findings: [] as Finding[],
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState(() => ({
				findings: asFindings(frame),
			})),
		reset: () =>
			setState(() => ({
				findings: [],
			})),
	}),
);
