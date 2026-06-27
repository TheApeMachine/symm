import { createStore } from "@tanstack/react-store";

export type WalkStep = {
	path: number[];
	outcome: "rejected" | "matched" | "parked" | "action";
	reason?: string;
};

export type WalkTrace = {
	symbol: string;
	steps: WalkStep[];
	active_path?: number[];
};

/*
playbookStore holds the latest per-symbol playbook descent traces broadcast by the
trader (role "walk"). Each frame carries `evaluations` keyed by symbol; the
Decision Tree surface renders which branches matched, parked, or were rejected.
*/
export const playbookStore = createStore(
	{ evaluations: {} as Record<string, WalkTrace> },
	({ setState }) => ({
		updateFrame: (frame: Record<string, unknown>) =>
			setState((prev) => {
				const evaluations = frame.evaluations as
					| Record<string, WalkTrace>
					| undefined;

					if (evaluations === undefined || evaluations === null) {
						return prev;
					}

					return {
						...prev,
						evaluations: {
							...prev.evaluations,
							...evaluations,
						},
					};
				}),
		}),
	);
