import { createStore } from "@tanstack/react-store";
import { Circular, type CircularBuffer } from "./circular";

export type DashboardFrame = Record<string, unknown>;
export type DashboardPayload = DashboardFrame | DashboardFrame[];

const DEFAULT_LIMIT = 256;

const ensureKeyedBuffer = (
	index: Record<string, CircularBuffer<DashboardFrame>>,
	key: string,
	limit: number,
): CircularBuffer<DashboardFrame> => {
	if (!index[key]) {
		index[key] = Circular<DashboardFrame>(limit);
	}

	return index[key];
};

const pushByKey = (
	index: Record<string, CircularBuffer<DashboardFrame>>,
	key: unknown,
	frame: DashboardFrame,
	limit: number,
): void => {
	if (typeof key !== "string" || key === "") {
		return;
	}

	ensureKeyedBuffer(index, key, limit).push(frame);
};

const ingestFrame = (
	state: {
		frame: DashboardFrame | null;
		frames: CircularBuffer<DashboardFrame>;
		bySymbol: Record<string, CircularBuffer<DashboardFrame>>;
		bySource: Record<string, CircularBuffer<DashboardFrame>>;
	},
	frame: DashboardFrame,
	limit: number,
): void => {
	state.frames.push(frame);
	state.frame = frame;
	pushByKey(state.bySymbol, frame.symbol, frame, limit);
	pushByKey(state.bySource, frame.source, frame, limit);
};

/*
createFrameCollection retains dashboard frames in bounded circular buffers so
websocket ingest does not grow unbounded arrays on every thesis tick.
*/
export const createFrameCollection = (limit = DEFAULT_LIMIT, merge = false) => {
	const frames = Circular<DashboardFrame>(limit);

	return createStore(
		{
			frame: null as DashboardFrame | null,
			frames,
			history: frames,
			bySymbol: {} as Record<string, CircularBuffer<DashboardFrame>>,
			bySource: {} as Record<string, CircularBuffer<DashboardFrame>>,
		},
		({ setState }) => ({
			updateFrame: (frame: DashboardPayload) => {
				if (Array.isArray(frame)) {
					if (frame.length === 0) {
						return;
					}

					setState((prev) => {
						for (const row of frame) {
							const merged =
								merge && prev.frame !== null ? { ...prev.frame, ...row } : row;

							ingestFrame(prev, merged, limit);
						}

						return { ...prev };
					});
					return;
				}

				setState((prev) => {
					const merged =
						merge && prev.frame !== null ? { ...prev.frame, ...frame } : frame;

					ingestFrame(prev, merged, limit);

					return { ...prev };
				});
			},
			updateFrames: (frames: DashboardFrame[]) => {
				if (frames.length === 0) {
					return;
				}

				setState((prev) => {
					for (const frame of frames) {
						ingestFrame(prev, frame, limit);
					}

					return { ...prev };
				});
			},
			reset: () => {
				const empty = Circular<DashboardFrame>(limit);

				setState(() => ({
					frame: null,
					frames: empty,
					history: empty,
					bySymbol: {},
					bySource: {},
				}));
			},
		}),
	);
};
