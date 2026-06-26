import { createStore } from "@tanstack/react-store";

export const decisionsStore = createStore(
	{ frame: null as Record<string, unknown> | null },
	({ setState }) => ({
		updateFrame: (frame: Record<string, unknown>) =>
			setState((prev) => {
				if (frame.role !== "decision") {
					return { ...prev, frame };
				}

				const prior = Array.isArray(prev.frame?.decisions)
					? (prev.frame.decisions as Array<Record<string, unknown>>)
					: [];
				const seq = frame.seq;
				const sameBatch = prev.frame?.seq === seq;
				const decisions = sameBatch ? [...prior, frame] : [frame];

				return {
					...prev,
					frame: {
						...frame,
						decisions,
					},
				};
			}),
	}),
);
