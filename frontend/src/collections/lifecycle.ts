import { createStore } from "@tanstack/react-store";
import type { LifecycleState } from "#/types/thesis";

/*
lifecycleStore mirrors the backend thesis.Lifecycle map so each symbol's state
machine position can be rendered without inferring it from execution frames.
*/
export const lifecycleStore = createStore(
	{
		lifecycle: {} as Record<string, LifecycleState>,
	},
	({ setState }) => ({
		updateFrame: (frame: Record<string, LifecycleState>) =>
			setState((prev) => ({
				lifecycle: {
					...prev.lifecycle,
					...frame,
				},
			})),
		reset: () =>
			setState(() => ({
				lifecycle: {},
			})),
	}),
);
