import { createStore } from "@tanstack/react-store";
import type { ArtifactFrame } from "#/collections/artifacts";

type DecisionFunnelState = {
	frame: ArtifactFrame | null;
	frames: ArtifactFrame[];
};

export const decisionFunnelStore = createStore(
	{ frame: null, frames: [] } as DecisionFunnelState,
	({ setState }) => ({
		updateFrames: (frames: ArtifactFrame[]) =>
			setState((prev) => {
				if (frames.length === 0) {
					return prev;
				}

				const next = [...prev.frames, ...frames].slice(-120);

				return {
					frame: next.at(-1) ?? null,
					frames: next,
				};
			}),
	}),
);
