import type { ArtifactFrame } from "#/collections/artifacts";

export type ScopedFrameMode = "concrete" | "stream_preview" | "missing";

export type ScopedFrameResult = {
	frame: ArtifactFrame | null;
	mode: ScopedFrameMode;
	sourceName: string;
	symbol: string;
};

export type ScopedFrameSource =
	| ArtifactFrame
	| ArtifactFrame[]
	| {
			frame?: ArtifactFrame | null;
			frames?: ArtifactFrame[];
			byScope?: Record<string, ArtifactFrame[]>;
	  }
	| Record<string, ArtifactFrame>
	| null
	| undefined;

export const isConcreteSymbol = (symbol: unknown): symbol is string =>
	typeof symbol === "string" && symbol.trim() !== "" && symbol !== "stream";

const asRecord = (value: unknown): ArtifactFrame | null =>
	value !== null && typeof value === "object" && !Array.isArray(value)
		? (value as ArtifactFrame)
		: null;

const recordArray = (value: unknown): ArtifactFrame[] =>
	Array.isArray(value)
		? value.flatMap((item) => {
				const record = asRecord(item);
				return record === null ? [] : [record];
			})
		: [];

const stringField = (frame: ArtifactFrame, key: string): string =>
	typeof frame[key] === "string" ? frame[key].trim() : "";

const frameDeclaresSymbol = (frame: ArtifactFrame, symbol: string): boolean =>
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
	frame: ArtifactFrame,
	symbol: string,
): ArtifactFrame | null => {
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

const streamPreviewFromRoot = (frame: ArtifactFrame): ArtifactFrame | null => {
	const focus = asRecord(frame.focus);

	if (focus !== null) {
		return focus;
	}

	const [firstSnapshot] = recordArray(frame.snapshots);

	return firstSnapshot ?? frame;
};

const looksLikeFrame = (record: ArtifactFrame): boolean =>
	[
		"role",
		"origin",
		"scope",
		"symbol",
		"focus",
		"focus_symbol",
		"snapshots",
		"layers",
		"output",
	].some((key) => record[key] !== undefined);

const collectFrames = (source: ScopedFrameSource): ArtifactFrame[] => {
	if (source === null || source === undefined) {
		return [];
	}

	if (Array.isArray(source)) {
		return source.flatMap((item) => collectFrames(item));
	}

	const record = asRecord(source);

	if (record === null) {
		return [];
	}

	const frames: ArtifactFrame[] = [];

	if (Array.isArray(record.frames)) {
		frames.push(...record.frames.flatMap((item) => collectFrames(item)));
	}

	const byScope = asRecord(record.byScope);

	if (byScope !== null) {
		for (const value of Object.values(byScope)) {
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
			const frame = scopedFrameFromRoot(frames[index] as ArtifactFrame, symbol);

			if (frame !== null) {
				return { frame, mode: "concrete", sourceName, symbol };
			}
		}

		return { frame: null, mode: "missing", sourceName, symbol };
	}

	for (let index = frames.length - 1; index >= 0; index -= 1) {
		const frame = streamPreviewFromRoot(frames[index] as ArtifactFrame);

		if (frame !== null) {
			return { frame, mode: "stream_preview", sourceName, symbol: "stream" };
		}
	}

	return { frame: null, mode: "missing", sourceName, symbol: "stream" };
};
