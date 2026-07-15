import { createStore } from "@tanstack/react-store";
import type { LifecycleState } from "#/types/thesis";

/*
lifecycleStore mirrors the backend thesis.Lifecycle map so each symbol's state
machine position can be rendered without inferring it from execution frames.
*/
export const lifecycleStore = createStore(
	{
		lifecycle: {} as Record<string, LifecycleState>,
		version: 0,
	},
	({ setState }) => ({
		updateFrame: (frame: Record<string, LifecycleState>) =>
			setState((prev) => {
				if (Object.keys(frame).length === 0) {
					return prev;
				}

				const lifecycle = prev.lifecycle;

				for (const [symbol, state] of Object.entries(frame)) {
					lifecycle[symbol] = state;
				}

				return {
					lifecycle,
					version: prev.version + 1,
				};
			}),
		reset: () =>
			setState(() => ({
				lifecycle: {},
				version: 0,
			})),
	}),
);
