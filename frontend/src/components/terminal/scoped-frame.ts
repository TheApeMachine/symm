import type { CircularBuffer } from "#/collections/circular";
import type { DashboardFrame } from "#/collections/frames";

export type ScopedFrameMode = "concrete" | "stream_preview" | "missing";

export type ScopedFrameResult = {
	frame: DashboardFrame | null;
	mode: ScopedFrameMode;
	sourceName: string;
	symbol: string;
};

export type ScopedFrameSource =
	| DashboardFrame
	| DashboardFrame[]
	| CircularBuffer<DashboardFrame>
	| {
			frame?: DashboardFrame | null;
			frames?: DashboardFrame[] | CircularBuffer<DashboardFrame>;
			bySymbol?: Record<
				string,
				DashboardFrame[] | CircularBuffer<DashboardFrame>
			>;
	  }
	| Record<string, DashboardFrame>
	| null
	| undefined;

const isCircularBuffer = (
	value: unknown,
): value is CircularBuffer<DashboardFrame> =>
	typeof value === "object" &&
	value !== null &&
	"values" in value &&
	typeof (value as CircularBuffer<DashboardFrame>).values === "function";

export const isConcreteSymbol = (symbol: unknown): symbol is string =>
	typeof symbol === "string" && symbol.trim() !== "" && symbol !== "stream";

const asRecord = (value: unknown): DashboardFrame | null =>
	value !== null && typeof value === "object" && !Array.isArray(value)
		? (value as DashboardFrame)
		: null;

const recordArray = (value: unknown): DashboardFrame[] =>
	Array.isArray(value)
		? value.flatMap((item) => {
				const record = asRecord(item);
				return record === null ? [] : [record];
			})
		: [];

const stringField = (frame: DashboardFrame, key: string): string =>
	typeof frame[key] === "string" ? frame[key].trim() : "";

const frameDeclaresSymbol = (frame: DashboardFrame, symbol: string): boolean =>
	[
		"symbol",
		"scope",
		"pair",
		"market",
		"focus_symbol",
		"symbol_pair",
		"instrument",
	].some((key) => stringField(frame, key) === symbol);

const scopedFrameFromRoot = (
	frame: DashboardFrame,
	symbol: string,
): DashboardFrame | null => {
	for (const snapshot of recordArray(frame.snapshots)) {
		if (frameDeclaresSymbol(snapshot, symbol)) {
			return snapshot;
		}
	}

	const focus = asRecord(frame.focus);

	if (focus !== null && frameDeclaresSymbol(focus, symbol)) {
		return focus;
	}

	return frameDeclaresSymbol(frame, symbol) ? frame : null;
};

const streamPreviewFromRoot = (
	frame: DashboardFrame,
): DashboardFrame | null => {
	const focus = asRecord(frame.focus);

	if (focus !== null) {
		return focus;
	}

	const [firstSnapshot] = recordArray(frame.snapshots);

	return firstSnapshot ?? frame;
};

const looksLikeFrame = (record: DashboardFrame): boolean =>
	[
		"role",
		"source",
		"scope",
		"symbol",
		"focus",
		"focus_symbol",
		"snapshots",
		"layers",
		"output",
	].some((key) => record[key] !== undefined);

const collectFrames = (source: ScopedFrameSource): DashboardFrame[] => {
	if (source === null || source === undefined) {
		return [];
	}

	if (Array.isArray(source)) {
		return source.flatMap((item) => collectFrames(item));
	}

	if (isCircularBuffer(source)) {
		return source.values().flatMap((item) => collectFrames(item));
	}

	const record = asRecord(source);

	if (record === null) {
		return [];
	}

	const frames: DashboardFrame[] = [];

	if (Array.isArray(record.frames)) {
		frames.push(...record.frames.flatMap((item) => collectFrames(item)));
	}

	if (isCircularBuffer(record.frames)) {
		frames.push(
			...record.frames.values().flatMap((item) => collectFrames(item)),
		);
	}

	const bySymbol = asRecord(record.bySymbol);

	if (bySymbol !== null) {
		for (const value of Object.values(bySymbol)) {
			if (isCircularBuffer(value)) {
				frames.push(...value.values().flatMap((item) => collectFrames(item)));
				continue;
			}

			frames.push(...collectFrames(value as ScopedFrameSource));
		}
	}

	const latest = asRecord(record.frame);

	if (latest !== null) {
		frames.push(latest);
	}

	if (frames.length === 0 && looksLikeFrame(record)) {
		frames.push(record);
	}

	if (frames.length === 0) {
		for (const [key, value] of Object.entries(record)) {
			const scopedFrames = collectFrames(value as ScopedFrameSource);

			if (isConcreteSymbol(key)) {
				frames.push(
					...scopedFrames.map((frame) =>
						frameDeclaresSymbol(frame, key) ? frame : { ...frame, scope: key },
					),
				);
				continue;
			}

			frames.push(...scopedFrames);
		}
	}

	return frames;
};

export const resolveScopedFrame = (
	source: ScopedFrameSource,
	concreteSymbol: string | null | undefined,
	sourceName: string,
): ScopedFrameResult => {
	const symbol = concreteSymbol?.trim() ?? "";
	const frames = collectFrames(source);
	const concrete = isConcreteSymbol(symbol);

	if (concrete) {
		for (let index = frames.length - 1; index >= 0; index -= 1) {
			const frame = scopedFrameFromRoot(
				frames[index] as DashboardFrame,
				symbol,
			);

			if (frame !== null) {
				return { frame, mode: "concrete", sourceName, symbol };
			}
		}

		return { frame: null, mode: "missing", sourceName, symbol };
	}

	for (let index = frames.length - 1; index >= 0; index -= 1) {
		const frame = streamPreviewFromRoot(frames[index] as DashboardFrame);

		if (frame !== null) {
			return { frame, mode: "stream_preview", sourceName, symbol: "stream" };
		}
	}

	return { frame: null, mode: "missing", sourceName, symbol: "stream" };
};
