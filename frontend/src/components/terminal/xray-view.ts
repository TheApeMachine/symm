import { semanticLayerName } from "#/components/terminal/xray-layers";
import { requirePositiveLength } from "#/lib/domain";
import { asFinite } from "#/lib/format";
import { parseResonanceData } from "#/lib/resonance";
import { symbolScopeAliases } from "#/lib/symbol-scope";
import { type Value as ResonanceFrame, Value } from "#/types/value";

export type HawkesMetrics = {
	intensity: number | null;
	branching: number | null;
	radius: number | null;
	asymmetry: number | null;
	buyIntensity: number | null;
	sellIntensity: number | null;
	exo: number | null;
};

export type LatentPoint = {
	key: string;
	symbol: string;
	x: number;
	y: number;
	category: string;
};

export type XrayLayer = {
	index: number;
	label: string;
	state: number[];
	prediction: number[];
	error_norm: number | null;
};

const stringValue = (value: unknown): string =>
	typeof value === "string" ? value.trim() : "";

const numberArray = (value: unknown): number[] =>
	Array.isArray(value)
		? value.filter((item): item is number => typeof item === "number")
		: [];

const valueEpochs = (values: Value[]): Value[][] => {
	const epochs = new Map<string, Value[]>();

	for (const val of values) {
		const key = String(val.timestamp);
		const epoch = epochs.get(key) ?? [];
		epoch.push(val);
		epochs.set(key, epoch);
	}

	return [...epochs.values()];
};

const valueRaw = (epoch: Value[], metric: string, side = ""): number | null => {
	const key = side === "" ? metric : `${metric}:${side}`;

	for (let index = epoch.length - 1; index >= 0; index -= 1) {
		const val = epoch[index];
		const raw = val.metric(key);

		if (typeof raw === "number" && Number.isFinite(raw)) {
			return raw;
		}
	}

	return null;
};

const layerError = (
	state: number[],
	prediction: number[],
	_surprise?: number,
): number | null => {
	if (state.length > 0 && prediction.length === state.length) {
		requirePositiveLength(state.length, "xray mean absolute error");

		const total = state.reduce(
			(sum, value, index) => sum + Math.abs(value - (prediction[index] ?? 0)),
			0,
		);

		return total / state.length;
	}

	return null;
};

/*
xrayLayersFromResonance maps backend resonance layers into hierarchy rows so the
predictive-coding panel never invents structure from manifold density.

The network reports its own prediction error per layer. That reading wins over
the mean absolute difference recomputed here, which only stands in for layers
published before errorNorm existed.
*/
export const xrayLayersFromResonance = (
	frame: ResonanceFrame | Record<string, unknown> | null | undefined,
): XrayLayer[] => {
	if (!frame) return [];

	/*
	A wire reading carries its hierarchy as layer_<i>_* scalar layers:
	parseResonanceData folds them into the layer rows the panel paints.
	*/
	const row =
		frame instanceof Value ? null : (frame as Record<string, unknown>);
	const parsed = row ? parseResonanceData(row) : undefined;

	const layers: Array<Record<string, unknown>> =
		parsed && parsed.layers.length > 0
			? (parsed.layers as unknown as Array<Record<string, unknown>>)
			: Array.isArray(row?.layers)
				? (row.layers as Array<Record<string, unknown>>)
				: [];

	const surprise =
		parsed?.surprise ??
		(typeof row?.surprise === "number" ? row.surprise : undefined);

	return layers.map((layer, index) => {
		const state = numberArray(layer.state);
		const prediction = numberArray(layer.prediction);
		const reported = asFinite(layer.errorNorm ?? layer.error_norm);

		return {
			index,
			label: `L${index} · ${semanticLayerName(index, layers.length)}`,
			state,
			prediction,
			error_norm: reported ?? layerError(state, prediction, surprise),
		};
	});
};

const hawkesMetrics = (epoch: Value[]): HawkesMetrics => {
	const aggregateIntensity =
		valueRaw(epoch, "conditional_intensity") ?? valueRaw(epoch, "arrival_rate");
	const buyIntensity =
		valueRaw(epoch, "conditional_intensity", "buy") ??
		valueRaw(epoch, "arrival_rate", "buy");
	const sellIntensity =
		valueRaw(epoch, "conditional_intensity", "sell") ??
		valueRaw(epoch, "arrival_rate", "sell");
	const radius =
		valueRaw(epoch, "branching_spectral_radius") ??
		valueRaw(epoch, "spectral_radius");

	const aggregateBg = valueRaw(epoch, "background_rate");
	const buyBg = valueRaw(epoch, "background_rate", "buy");
	const sellBg = valueRaw(epoch, "background_rate", "sell");

	const intensity =
		aggregateIntensity ??
		(buyIntensity !== null && sellIntensity !== null
			? buyIntensity + sellIntensity
			: (buyIntensity ?? sellIntensity));

	const exo =
		aggregateBg ??
		(buyBg !== null && sellBg !== null ? buyBg + sellBg : (buyBg ?? sellBg));

	return {
		intensity,
		branching: radius,
		radius,
		asymmetry: null,
		buyIntensity,
		sellIntensity,
		exo,
	};
};

/*
hawkesMetricsFromFrames reads Hawkes intensity metrics from a flat value
history for one symbol/source without store access.
*/
export const hawkesMetricsFromFrames = (frames: Value[]): HawkesMetrics => {
	const epochs = valueEpochs(frames);
	const epoch = epochs
		.sort((left, right) =>
			Number((left[0]?.timestamp ?? 0n) - (right[0]?.timestamp ?? 0n)),
		)
		.at(-1);

	if (epoch === undefined) {
		return {
			intensity: null,
			branching: null,
			radius: null,
			asymmetry: null,
			buyIntensity: null,
			sellIntensity: null,
			exo: null,
		};
	}

	return hawkesMetrics(epoch);
};

/*
hawkesMetricsFromBuffer is the array-facing alias used by paint paths that
already filtered Hawkes rows from a worker snapshot.
*/
export const hawkesMetricsFromBuffer = (
	frames: Value[] | undefined,
): HawkesMetrics => hawkesMetricsFromFrames(frames ?? []);

export const hawkesEventCount = (frames: Value[] | undefined): number =>
	frames?.length ?? 0;

/*
intensitySeriesFromRingRows collapses per-epoch intensity samples into a
plotted series, sorted by arrival. A ring buffer holds one row per emission,
not one row per epoch, and most emissions for an epoch carry an unrelated
Hawkes binding (compensator, excitation decay, offspring, ...) rather than
intensity — a caller that pushed a placeholder for every row lacking the
metric turned a sparse impulse train into linearly-interpolated triangles.
Only epochs that actually reported the metric are included, keeping the last
sample per epoch when more than one row for the same epoch reports it.
*/
export const intensitySeriesFromRingRows = (
	rows: Array<{ at: bigint; raw: number }>,
): number[] => {
	const byEpoch = new Map<bigint, number>();

	for (const row of rows) {
		byEpoch.set(row.at, row.raw);
	}

	return [...byEpoch.entries()]
		.sort((left, right) =>
			left[0] < right[0] ? -1 : left[0] > right[0] ? 1 : 0,
		)
		.map(([, value]) => value);
};

/*
latentPointsFromFrames projects each symbol's latest resonance latent pair for
the universe scatter without inventing coordinates.
*/
export const latentPointsFromFrames = (
	frames: Array<ResonanceFrame | Record<string, unknown>>,
): LatentPoint[] => {
	const latest = new Map<string, ResonanceFrame | Record<string, unknown>>();

	for (const frame of frames) {
		// A wire reading scopes itself to its symbol.
		const symbol = stringValue(
			frame instanceof Value
				? frame.scope
				: (frame as Record<string, unknown>).symbol,
		);

		if (symbol !== "") {
			latest.set(symbol, frame);
		}
	}

	return [...latest.entries()].flatMap(([symbol, frame]) => {
		/*
		The reading carries its latent state as latent_<i> scalar layers;
		the leading two components place the symbol in the universe
		cross-section.
		*/
		const row = frame as Record<string, unknown>;
		const parsed = frame instanceof Value ? null : parseResonanceData(row);
		const position = parsed?.latent?.length
			? parsed.latent.slice(0, 2)
			: numberArray(row.latent ?? row.embedding);

		if (position.length < 2) {
			return [];
		}

		const at =
			frame instanceof Value
				? String(frame.tick)
				: stringValue(row.at ?? row.tick);

		return [
			{
				key: `${symbol}:${at}`,
				symbol,
				x: position[0] ?? 0,
				y: position[1] ?? 0,
				category: stringValue(row.category),
			},
		];
	});
};

export const carrierScope = (
	frame: ResonanceFrame | Record<string, unknown> | Value,
): string => {
	if (frame instanceof Value) return frame.scope;
	const row = frame as Record<string, unknown>;
	return stringValue(row.symbol ?? row.scope);
};

/*
resolveXrayCarrier picks the focus symbol's row when it has hierarchy layers,
otherwise the best available carrier in the live universe.
*/
export const resolveXrayCarrier = (
	frames: Array<ResonanceFrame | Record<string, unknown> | Value>,
	focusSymbol: string,
): ResonanceFrame | Record<string, unknown> | Value | undefined => {
	if (frames.length === 0) return undefined;

	const aliases = new Set(symbolScopeAliases(focusSymbol));
	const direct = frames.find((frame) => aliases.has(carrierScope(frame)));

	if (direct && xrayLayersFromResonance(direct).length > 0) {
		return direct;
	}

	for (const frame of frames) {
		if (xrayLayersFromResonance(frame).length > 0) {
			return frame;
		}
	}

	return direct ?? frames[0];
};
