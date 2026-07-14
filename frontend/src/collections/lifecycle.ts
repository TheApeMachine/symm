import { createStore } from "@tanstack/react-store";
import type { LifecycleState } from "#/types/thesis";

const asLifecycle = (frame: unknown): Record<string, LifecycleState> => {
	if (typeof frame !== "object" || frame === null || Array.isArray(frame)) {
		return {};
	}

	const lifecycle: Record<string, LifecycleState> = {};

	for (const [symbol, state] of Object.entries(frame)) {
		if (typeof state === "string" && state.length > 0) {
			lifecycle[symbol] = state;
		}
	}

	return lifecycle;
};

/*
lifecycleStore mirrors the backend thesis.Lifecycle map so each symbol's state
machine position can be rendered without inferring it from execution frames.
*/
export const lifecycleStore = createStore(
	{
		lifecycle: {} as Record<string, LifecycleState>,
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => ({
				lifecycle: {
					...prev.lifecycle,
					...asLifecycle(frame),
				},
			})),
		reset: () =>
			setState(() => ({
				lifecycle: {},
			})),
	}),
);
