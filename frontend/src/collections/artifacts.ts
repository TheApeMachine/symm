import { createStore } from "@tanstack/react-store";

export type ArtifactFrame = Record<string, unknown>;

const DEFAULT_LIMIT = 256;

export const boundedAppend = (
	frames: ArtifactFrame[],
	frame: ArtifactFrame,
	limit = DEFAULT_LIMIT,
): ArtifactFrame[] => [...frames, frame].slice(-Math.max(1, limit));

export const appendByKey = (
	index: Record<string, ArtifactFrame[]>,
	key: unknown,
	frame: ArtifactFrame,
	limit = DEFAULT_LIMIT,
): Record<string, ArtifactFrame[]> => {
	if (typeof key !== "string" || key === "") {
		return index;
	}

	return {
		...index,
		[key]: boundedAppend(index[key] ?? [], frame, limit),
	};
};

export const createArtifactCollection = (limit = DEFAULT_LIMIT) =>
	createStore(
		{
			frame: null as ArtifactFrame | null,
			frames: [] as ArtifactFrame[],
			history: [] as ArtifactFrame[],
			byScope: {} as Record<string, ArtifactFrame[]>,
			byOrigin: {} as Record<string, ArtifactFrame[]>,
		},
		({ setState }) => ({
			updateFrame: (frame: ArtifactFrame) =>
				setState((prev) => {
					const frames = boundedAppend(prev.frames, frame, limit);

					return {
						frame,
						frames,
						history: frames,
						byScope: appendByKey(prev.byScope, frame.scope, frame, limit),
						byOrigin: appendByKey(prev.byOrigin, frame.origin, frame, limit),
					};
				}),
			updateFrames: (frames: ArtifactFrame[]) => {
				if (frames.length === 0) {
					return;
				}

				setState((prev) => {
					let next = prev;

					for (const frame of frames) {
						const nextFrames = boundedAppend(next.frames, frame, limit);
						next = {
							frame,
							frames: nextFrames,
							history: nextFrames,
							byScope: appendByKey(next.byScope, frame.scope, frame, limit),
							byOrigin: appendByKey(next.byOrigin, frame.origin, frame, limit),
						};
					}

					return next;
				});
			},
			reset: () =>
				setState(() => ({
					frame: null,
					frames: [],
					history: [],
					byScope: {},
					byOrigin: {},
				})),
		}),
	);
