import { createStore } from "@tanstack/react-store";

export const manifoldStore = createStore(
	{
		frame: null as Record<string, unknown> | null,
		frames: [] as Record<string, unknown>[],
	},
	({ setState }) => ({
		updateFrame: (frame: Record<string, unknown>) =>
			setState((prev) => ({
				frame,
				frames: [...prev.frames, frame].slice(-50),
			})),
	}),
);
