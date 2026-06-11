import { createStore } from "@tanstack/react-store";

export type PlaybookBranch = {
	condition_group?: {
		boolean?: string;
		conditions?: Array<{
			type?: string;
			left?: { subject?: Record<string, unknown> };
			right?: { subject?: Record<string, unknown> };
		}>;
	};
	action?: {
		type?: string;
		side?: string;
		fraction?: number;
	};
	branches?: PlaybookBranch[];
};

export const playbookStore = createStore(
	{
		branches: [] as PlaybookBranch[],
	},
	({ setState }) => ({
		updateBranches: (branches: PlaybookBranch[]) =>
			setState((prev) => ({
				...prev,
				branches: branches,
			})),
	}),
);
