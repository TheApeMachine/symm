export type PredictionPoint = {
	x: number;
	value: number;
};

export type PredictionSeries = {
	actual: PredictionPoint[];
	prediction: PredictionPoint[];
	error: PredictionPoint[];
};

export type TerminalResonanceLayer = {
	state: number[];
	prediction: number[];
	errorNorm: number;
};

export type TerminalResonanceFrame = {
	symbol: string;
	category: string;
	surprise: number;
	energy: number;
	confidence: number;
	layers: TerminalResonanceLayer[];
};

const SERIES_LIMIT = 160;
const FLUID_COLUMNS = 64;
const FLUID_ROWS = 38;
const FLUID_METRICS = ["volume", "change_pct", "re", "div", "vort", "turb"];

const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const finiteArray = (value: unknown): number[] => {
	if (!Array.isArray(value)) {
		return [];
	}

	return value.filter(
		(entry): entry is number =>
			typeof entry === "number" && Number.isFinite(entry),
	);
};

const rowNumber = (row: unknown, key: string): number | null => {
	if (typeof row !== "object" || row === null) {
		return null;
	}

	const value = (row as Record<string, unknown>)[key];

	return finiteNumber(value);
};

const predictionKind = (value: unknown): keyof PredictionSeries | null => {
	if (value === "actual" || value === "prediction" || value === "error") {
		return value;
	}

	return null;
};

const upsertPoint = (
	points: PredictionPoint[],
	point: PredictionPoint,
): PredictionPoint[] => {
	const next = points.filter((entry) => entry.x !== point.x);
	next.push(point);
	next.sort((left, right) => left.x - right.x);

	return next.slice(-SERIES_LIMIT);
};

export const appendPredictionFrame = (
	series: PredictionSeries,
	frame: Record<string, unknown>,
): PredictionSeries => {
	const kind = predictionKind(frame.kind);
	const x = finiteNumber(frame.x);
	const value = finiteNumber(frame.value);

	if (kind === null || x === null || value === null) {
		return series;
	}

	return {
		...series,
		[kind]: upsertPoint(series[kind], { x, value }),
	};
};

export const emptyPredictionSeries = (): PredictionSeries => ({
	actual: [],
	prediction: [],
	error: [],
});

const emptyMatrix = (rows: number, columns: number): number[][] =>
	Array.from({ length: rows }, () => Array.from({ length: columns }, () => 0));

const rank = (value: number, sorted: number[]): number => {
	if (sorted.length <= 1) {
		return 0.5;
	}

	let below = 0;

	for (const entry of sorted) {
		if (entry < value) {
			below += 1;
		}
	}

	return below / (sorted.length - 1);
};

const clampIndex = (value: number, upper: number): number =>
	Math.max(0, Math.min(upper - 1, value));

const metricValue = (row: unknown, key: string): number | null => {
	if (key === "volume") {
		return rowNumber(row, "volume") ?? rowNumber(row, "vol");
	}

	return rowNumber(row, key);
};

const metricMaxima = (rows: unknown[]) =>
	Object.fromEntries(
		FLUID_METRICS.map((key) => [
			key,
			Math.max(
				0,
				...rows
					.map((row) => metricValue(row, key))
					.filter((value): value is number => value !== null)
					.map(Math.abs),
			),
		]),
	);

const activity = (
	row: unknown,
	maxima: Record<string, number>,
): number | null => {
	const values = FLUID_METRICS.flatMap((key) => {
		const value = metricValue(row, key);
		const max = maxima[key] ?? 0;

		if (value === null || max <= 0) {
			return [];
		}

		return [Math.abs(value) / max];
	});

	if (values.length === 0) {
		return null;
	}

	return values.reduce((sum, value) => sum + value, 0) / values.length;
};

const addField = (
	matrix: number[][],
	column: number,
	row: number,
	value: number,
	radius: number,
) => {
	for (let rowOffset = -radius; rowOffset <= radius; rowOffset += 1) {
		for (
			let columnOffset = -radius;
			columnOffset <= radius;
			columnOffset += 1
		) {
			const targetRow = row + rowOffset;
			const targetColumn = column + columnOffset;

			if (
				targetRow < 0 ||
				targetColumn < 0 ||
				targetRow >= matrix.length ||
				targetColumn >= (matrix[0]?.length ?? 0)
			) {
				continue;
			}

			const distance = Math.hypot(rowOffset, columnOffset);

			if (distance > radius) {
				continue;
			}

			matrix[targetRow][targetColumn] += value / (1 + distance);
		}
	}
};

export type TerminalFluidFieldStats = {
	gridText: string;
	outliersText: string;
	peakText: string;
	outliers: number;
	peak: string;
	focusSymbol: string;
};

export const terminalFluidFieldStats = (
	frame: Record<string, unknown> | null,
	matrix: number[][],
): TerminalFluidFieldStats => {
	const symbols = frame?.symbols;

	if (!Array.isArray(symbols) || symbols.length === 0) {
		return {
			gridText: "64 × 38",
			outliersText: "0 outliers",
			peakText: "peak 0.00",
			outliers: 0,
			peak: "0.00",
			focusSymbol: "market",
		};
	}

	const changes = symbols
		.map((row) => metricValue(row, "change_pct"))
		.filter((value): value is number => value !== null);
	const mean =
		changes.length > 0
			? changes.reduce((sum, value) => sum + value, 0) / changes.length
			: 0;
	const outliers = changes.filter(
		(value) => Math.abs(value - mean) > Math.max(Math.abs(mean) * 0.75, 0.35),
	).length;
	const flat = matrix.flat().filter(Number.isFinite);
	const peak = flat.length > 0 ? Math.max(...flat) : 0;
	const focusRow = symbols[0];
	const focusSymbol =
		typeof focusRow === "object" &&
		focusRow !== null &&
		typeof (focusRow as Record<string, unknown>).symbol === "string"
			? ((focusRow as Record<string, unknown>).symbol as string)
			: "market";

	return {
		gridText: `${FLUID_COLUMNS} × ${FLUID_ROWS}`,
		outliersText: `${outliers} outlier${outliers === 1 ? "" : "s"}`,
		peakText: `peak ${peak.toFixed(2)}`,
		outliers,
		peak: peak.toFixed(2),
		focusSymbol,
	};
};

export const terminalFluidMatrix = (
	frame: Record<string, unknown>,
): number[][] => {
	const symbols = frame.symbols;

	if (!Array.isArray(symbols) || symbols.length === 0) {
		return [];
	}

	const matrix = emptyMatrix(FLUID_ROWS, FLUID_COLUMNS);
	const volumes = symbols
		.map((row) => metricValue(row, "volume"))
		.filter((value): value is number => value !== null)
		.sort((left, right) => left - right);
	const changes = symbols
		.map((row) => metricValue(row, "change_pct"))
		.filter((value): value is number => value !== null)
		.sort((left, right) => left - right);
	const maxima = metricMaxima(symbols);
	const radius = Math.max(
		1,
		Math.round(Math.sqrt((FLUID_COLUMNS * FLUID_ROWS) / symbols.length) / 2),
	);

	for (const [index, symbol] of symbols.entries()) {
		const volume = metricValue(symbol, "volume");
		const change = metricValue(symbol, "change_pct") ?? 0;
		const value = activity(symbol, maxima);

		if (value === null) {
			continue;
		}

		const column = clampIndex(
			volume === null
				? Math.round((index / Math.max(symbols.length - 1, 1)) * FLUID_COLUMNS)
				: Math.round(rank(volume, volumes) * FLUID_COLUMNS),
			FLUID_COLUMNS,
		);
		const row = clampIndex(
			Math.round((1 - rank(change, changes)) * FLUID_ROWS),
			FLUID_ROWS,
		);
		addField(matrix, column, row, value, radius);
	}

	return matrix;
};

const numericMatrix = (value: unknown): number[][] => {
	if (!Array.isArray(value)) {
		return [];
	}

	return value
		.map((row) => (Array.isArray(row) ? finiteArray(row) : []))
		.filter((row) => row.length > 0);
};

const normalizeMatrix = (matrix: number[][]): number[][] => {
	if (matrix.length === 0) {
		return [];
	}

	const values = matrix.flat();
	const min = Math.min(...values);
	const max = Math.max(...values);
	const span = max > min ? max - min : 1;

	return matrix.map((row) => row.map((value) => (value - min) / span));
};

export const terminalManifoldMatrix = (
	frame: Record<string, unknown> | null,
): number[][] => {
	if (frame === null) {
		return [];
	}

	return normalizeMatrix(numericMatrix(frame.rho));
};

const parseLayer = (value: unknown): TerminalResonanceLayer | null => {
	if (typeof value !== "object" || value === null) {
		return null;
	}

	const layer = value as Record<string, unknown>;
	const state = finiteArray(layer.state);

	if (state.length === 0) {
		return null;
	}

	return {
		state,
		prediction: finiteArray(layer.prediction),
		errorNorm:
			finiteNumber(layer.error_norm) ?? finiteNumber(layer.errorNorm) ?? 0,
	};
};

const parseResonanceXRay = (
	raw: Record<string, unknown>,
): TerminalResonanceFrame | null => {
	const symbol = typeof raw.symbol === "string" ? raw.symbol.trim() : "";
	const layersRaw = raw.layers;

	if (symbol === "" || !Array.isArray(layersRaw)) {
		return null;
	}

	const layers = layersRaw.flatMap((layer) => {
		const parsed = parseLayer(layer);

		return parsed === null ? [] : [parsed];
	});

	if (layers.length === 0) {
		return null;
	}

	return {
		symbol,
		category: typeof raw.category === "string" ? raw.category : "",
		surprise: finiteNumber(raw.surprise) ?? 0,
		energy: finiteNumber(raw.energy) ?? 0,
		confidence: finiteNumber(raw.confidence) ?? 0,
		layers,
	};
};

import { parseResonanceUniverseFrame } from "#/components/charts/resonance/resonance-universe-frame";

export const terminalResonanceFrame = (
	frame: Record<string, unknown> | null,
	focusSymbol?: string,
): TerminalResonanceFrame | null => {
	if (frame === null) {
		return null;
	}

	if (frame.type === "resonance_universe") {
		const universe = parseResonanceUniverseFrame(frame);

		if (universe === null) {
			return null;
		}

		const scope = focusSymbol?.trim() || universe.focusSymbol;

		if (scope === universe.focus.symbol) {
			return parseResonanceXRay(
				universe.focus as unknown as Record<string, unknown>,
			);
		}

		const summary = universe.symbols.find((row) => row.symbol === scope);

		if (summary === undefined) {
			return null;
		}

		return {
			symbol: summary.symbol,
			category: summary.category,
			surprise: summary.surprise,
			energy: summary.energy,
			confidence: summary.confidence,
			layers: [],
		};
	}

	return parseResonanceXRay(frame);
};

export type TerminalManifoldCarrier = {
	symbol: string;
	lx: number;
	ly: number;
	vol: number;
	cluster: number;
};

export type TerminalManifoldReading = {
	divergence: string;
	coherence: string;
	guidance: string;
	viscosity: string;
	momentumShare: string;
	momentumPct: number;
	momentumGate: string | null;
	momentumGatePct: number | null;
	momentumFg: string;
};

export type HawkesWireMetrics = {
	intensity: number;
	branching: number;
	buyIntensity: number;
	sellIntensity: number;
	alpha: number;
	beta: number;
	eta: number;
	baseline: number;
};

const nestedRecord = (value: unknown): Record<string, unknown> | null =>
	typeof value === "object" && value !== null
		? (value as Record<string, unknown>)
		: null;

const nestedWireNumber = (
	frame: Record<string, unknown>,
	...path: string[]
): number | null => {
	let current: unknown = frame;

	for (const segment of path) {
		const record = nestedRecord(current);

		if (record === null) {
			return null;
		}

		current = record[segment];
	}

	return finiteNumber(current);
};

export const hawkesWireMetrics = (
	frame: Record<string, unknown> | null | undefined,
): HawkesWireMetrics | null => {
	if (frame === undefined || frame === null) {
		return null;
	}

	const intensity =
		nestedWireNumber(frame, "output", "intensity") ??
		finiteNumber(frame.intensity);
	const branching =
		nestedWireNumber(frame, "output", "branching") ??
		finiteNumber(frame.branching);
	const buyIntensity =
		nestedWireNumber(frame, "output", "buyIntensity") ??
		finiteNumber(frame.buyIntensity);
	const sellIntensity =
		nestedWireNumber(frame, "output", "sellIntensity") ??
		finiteNumber(frame.sellIntensity);

	if (
		intensity === null &&
		branching === null &&
		buyIntensity === null &&
		sellIntensity === null
	) {
		return null;
	}

	const resolvedIntensity =
		intensity ?? (buyIntensity ?? 0) + (sellIntensity ?? 0);
	const resolvedBranching = branching ?? 0;
	const resolvedBuy = buyIntensity ?? 0;
	const resolvedSell = sellIntensity ?? 0;
	const wireAlpha =
		nestedWireNumber(frame, "output", "alpha") ?? finiteNumber(frame.alpha);
	const wireBeta =
		nestedWireNumber(frame, "output", "beta") ?? finiteNumber(frame.beta);
	const resolvedBeta =
		wireBeta ??
		Math.max(
			resolvedSell > 0
				? resolvedSell
				: resolvedIntensity > 0
					? resolvedIntensity
					: resolvedBuy,
			0.1,
		);
	const resolvedAlpha =
		wireAlpha ??
		(resolvedBranching > 0
			? resolvedBranching * resolvedBeta
			: resolvedBuy > 0
				? resolvedBuy
				: resolvedIntensity * 0.35);
	const baseline =
		resolvedIntensity > 0
			? resolvedIntensity / Math.max(resolvedBranching, 1)
			: 0;

	return {
		intensity: resolvedIntensity,
		branching: resolvedBranching,
		buyIntensity: resolvedBuy,
		sellIntensity: resolvedSell,
		alpha: resolvedAlpha,
		beta: resolvedBeta,
		eta: resolvedBeta > 0 ? resolvedAlpha / resolvedBeta : resolvedBranching,
		baseline: baseline > 0 ? baseline : resolvedIntensity * 0.35,
	};
};

export const terminalManifoldCarriers = (
	frame: Record<string, unknown> | null,
	_focusSymbol: string,
): TerminalManifoldCarrier[] => {
	const carriers = frame?.carriers;

	if (!Array.isArray(carriers) || carriers.length === 0) {
		return [];
	}

	return carriers
		.flatMap((row, index) => {
			if (typeof row !== "object" || row === null) {
				return [];
			}

			const carrier = row as Record<string, unknown>;
			const symbol =
				typeof carrier.symbol === "string" ? carrier.symbol.trim() : "";
			const x = finiteNumber(carrier.x);
			const y = finiteNumber(carrier.y);
			const heat =
				finiteNumber(carrier.heat) ?? finiteNumber(carrier.amplitude);

			if (symbol === "" || x === null || y === null) {
				return [];
			}

			const role = typeof carrier.role === "string" ? carrier.role : "";
			const cluster = role === "whale" ? 1 : role === "carrier" ? 0 : index % 4;

			return [
				{
					symbol,
					lx: x,
					ly: y,
					vol: heat ?? 0.5,
					cluster,
				},
			];
		})
		.filter((point) => {
			const base = point.symbol.split("/")[0] ?? point.symbol;

			return base.length > 0;
		});
};

export const terminalManifoldReading = (
	frame: Record<string, unknown> | null,
): TerminalManifoldReading | null => {
	const reading = nestedRecord(frame?.reading);

	if (reading === null) {
		return null;
	}

	const divergence = finiteNumber(reading.divergence);
	const coherence = finiteNumber(reading.coherence_mag2);
	const guidance = finiteNumber(reading.guidance_speed);
	const viscosity = finiteNumber(reading.viscosity_proxy);
	const momentum =
		finiteNumber(reading.momentum_share) ??
		finiteNumber(reading.pressure_grad_norm);

	if (
		divergence === null &&
		coherence === null &&
		guidance === null &&
		viscosity === null &&
		momentum === null
	) {
		return null;
	}

	const share = Math.max(0, Math.min(1, momentum ?? 0));
	const gate =
		finiteNumber(reading.mode_share) ??
		finiteNumber(reading.momentum_gate) ??
		finiteNumber(reading.gate);

	return {
		divergence:
			divergence === null
				? "—"
				: `${divergence >= 0 ? "+" : "−"}${Math.abs(divergence).toFixed(3)}`,
		coherence: coherence?.toFixed(3) ?? "—",
		guidance: guidance?.toFixed(3) ?? "—",
		viscosity: viscosity?.toFixed(3) ?? "—",
		momentumShare: share.toFixed(2),
		momentumPct: Math.round(share * 100),
		momentumGate: gate === null ? null : gate.toFixed(2),
		momentumGatePct: gate === null ? null : Math.round(gate * 100),
		momentumFg: gate !== null && share >= gate ? "var(--up)" : "var(--f3)",
	};
};
