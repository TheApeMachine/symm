import { createStore } from "@tanstack/react-store";

export type DashboardFrame = Record<string, unknown>;
export type DashboardPayload = DashboardFrame | DashboardFrame[];

const DEFAULT_LIMIT = 256;

export const boundedAppend = (
	frames: DashboardFrame[],
	frame: DashboardFrame,
	limit = DEFAULT_LIMIT,
): DashboardFrame[] => [...frames, frame].slice(-Math.max(1, limit));

export const appendByKey = (
	index: Record<string, DashboardFrame[]>,
	key: unknown,
	frame: DashboardFrame,
	limit = DEFAULT_LIMIT,
): Record<string, DashboardFrame[]> => {
	if (typeof key !== "string" || key === "") {
		return index;
	}

	return {
		...index,
		[key]: boundedAppend(index[key] ?? [], frame, limit),
	};
};

export const createFrameCollection = (limit = DEFAULT_LIMIT) =>
	createStore(
		{
			frame: null as DashboardFrame | null,
			frames: [] as DashboardFrame[],
			history: [] as DashboardFrame[],
			bySymbol: {} as Record<string, DashboardFrame[]>,
			bySource: {} as Record<string, DashboardFrame[]>,
		},
		({ setState }) => ({
			updateFrame: (frame: DashboardPayload) => {
				if (Array.isArray(frame)) {
					if (frame.length === 0) {
						return;
					}

					setState((prev) => {
						let next = prev;

						for (const row of frame) {
							const frames = boundedAppend(next.frames, row, limit);
							next = {
								frame: row,
								frames,
								history: frames,
								bySymbol: appendByKey(next.bySymbol, row.symbol, row, limit),
								bySource: appendByKey(next.bySource, row.source, row, limit),
							};
						}

						return next;
					});
					return;
				}

				setState((prev) => {
					const frames = boundedAppend(prev.frames, frame, limit);

					return {
						frame,
						frames,
						history: frames,
						bySymbol: appendByKey(prev.bySymbol, frame.symbol, frame, limit),
						bySource: appendByKey(prev.bySource, frame.source, frame, limit),
					};
				});
			},
			updateFrames: (frames: DashboardFrame[]) => {
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
							bySymbol: appendByKey(next.bySymbol, frame.symbol, frame, limit),
							bySource: appendByKey(next.bySource, frame.source, frame, limit),
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
					bySymbol: {},
					bySource: {},
				})),
		}),
	);
