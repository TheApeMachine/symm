import { createStore } from "@tanstack/react-store";

export const positionsStore = createStore(
	{ frame: null as Record<string, unknown> | null },
	({ setState }) => ({
		updateFrame: (frame: Record<string, unknown>) =>
			setState((prev) => ({ ...prev, frame })),
	}),
);
