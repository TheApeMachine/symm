import { createStore } from "@tanstack/react-store";
import {
	type ArtifactFrame,
	appendByKey,
	boundedAppend,
} from "#/collections/artifacts";

export type MeasurementHistorySample = ArtifactFrame;

type MeasurementsState = Record<string, Record<string, ArtifactFrame>>;

type MeasurementMeta = {
	frame: ArtifactFrame | null;
	frames: ArtifactFrame[];
	history: ArtifactFrame[];
	byScope: Record<string, ArtifactFrame[]>;
	byOrigin: Record<string, ArtifactFrame[]>;
	byOriginScope: Record<string, Record<string, ArtifactFrame[]>>;
};

export type MeasurementsCollectionState = MeasurementsState & MeasurementMeta;

const LIMIT = 256;

const withMeta = (
	state: Record<string, Record<string, ArtifactFrame>>,
	meta: MeasurementMeta,
): MeasurementsState =>
	Object.defineProperties(state, {
		frame: { value: meta.frame, enumerable: false, configurable: true },
		frames: { value: meta.frames, enumerable: false, configurable: true },
		history: { value: meta.history, enumerable: false, configurable: true },
		byScope: { value: meta.byScope, enumerable: false, configurable: true },
		byOrigin: { value: meta.byOrigin, enumerable: false, configurable: true },
		byOriginScope: {
			value: meta.byOriginScope,
			enumerable: false,
			configurable: true,
		},
	}) as MeasurementsState;

const emptyState = (): MeasurementsState =>
	withMeta(
		{},
		{
			frame: null,
			frames: [],
			history: [],
			byScope: {},
			byOrigin: {},
			byOriginScope: {},
		},
	);

const appendByOriginScope = (
	index: Record<string, Record<string, ArtifactFrame[]>>,
	origin: string,
	scope: string,
	frame: ArtifactFrame,
): Record<string, Record<string, ArtifactFrame[]>> => ({
	...index,
	[origin]: {
		...(index[origin] ?? {}),
		[scope]: boundedAppend(index[origin]?.[scope] ?? [], frame, LIMIT),
	},
});

export const measurementsStore = createStore(emptyState(), ({ setState }) => ({
	updateReading: (frame: ArtifactFrame) =>
		measurementsStore.actions.updateReadings([frame]),
	updateReadings: (frames: ArtifactFrame[]) => {
		if (frames.length === 0) {
			return;
		}

		setState((prev) => {
			const next: Record<string, Record<string, ArtifactFrame>> = { ...prev };
			const meta = prev as MeasurementsCollectionState;
			let latest = meta.frame;
			let allFrames = meta.frames;
			let byScope = meta.byScope;
			let byOrigin = meta.byOrigin;
			let byOriginScope = meta.byOriginScope;
			const touched = new Set<string>();

			for (const frame of frames) {
				const origin = typeof frame.origin === "string" ? frame.origin : "";
				const scope = typeof frame.scope === "string" ? frame.scope : "";

				if (origin === "" || scope === "") {
					continue;
				}

				const bySymbol =
					next[origin] === undefined || touched.has(origin)
						? (next[origin] ?? {})
						: { ...next[origin] };

				bySymbol[scope] = frame;
				next[origin] = bySymbol;
				touched.add(origin);

				latest = frame;
				allFrames = boundedAppend(allFrames, frame, LIMIT);
				byScope = appendByKey(byScope, scope, frame, LIMIT);
				byOrigin = appendByKey(byOrigin, origin, frame, LIMIT);
				byOriginScope = appendByOriginScope(
					byOriginScope,
					origin,
					scope,
					frame,
				);
			}

			if (touched.size === 0) {
				return prev;
			}

			return withMeta(next, {
				frame: latest,
				frames: allFrames,
				history: allFrames,
				byScope,
				byOrigin,
				byOriginScope,
			});
		});
	},
	reset: () => {
		setState(() => emptyState());
	},
}));
