import { kernelRowAt } from "#/lib/kernel-row";

export type LayerData = {
	state: number[];
	prediction: number[];
	errorNorm?: number;
};

export type ResonanceData = {
	taskRelativePrecision?: number;
	taskSkill?: number;
	supportedHorizon?: number;
	surprise?: number;
	surpriseBaseline?: number;
	surpriseRegimeZ?: number;
	surpriseRank?: number;
	energy?: number;
	calibrated?: boolean;
	forwardCurve: number[];
	latent: number[];
	layers: LayerData[];
};

const densify = (sparse: number[]): number[] => {
	if (sparse.length === 0) return [];
	const out: number[] = [];
	for (let i = 0; i < sparse.length; i++) {
		const v = sparse[i];
		out.push(typeof v === "number" && Number.isFinite(v) ? v : 0);
	}
	return out;
};

const finalize = (data: ResonanceData): ResonanceData => ({
	...data,
	forwardCurve: densify(data.forwardCurve),
	latent: densify(data.latent),
	layers: data.layers
		.filter((layer) => layer != null)
		.map((layer) => ({
			state: densify(layer.state),
			prediction: densify(layer.prediction),
			errorNorm: layer.errorNorm,
		})),
});

const applyKey = (data: ResonanceData, key: string, value: unknown) => {
	if (typeof value !== "number" || !Number.isFinite(value)) return;

	switch (key) {
		case "task_relative_precision":
			data.taskRelativePrecision = value;
			return;
		case "task_skill":
			data.taskSkill = value;
			return;
		case "surprise":
			data.surprise = value;
			return;
		case "surprise_baseline":
			data.surpriseBaseline = value;
			return;
		case "surprise_regime_z":
			data.surpriseRegimeZ = value;
			return;
		case "surprise_rank":
			data.surpriseRank = value;
			return;
		case "energy":
			data.energy = value;
			return;
		case "supported_horizon":
			data.supportedHorizon = Math.trunc(value);
			return;
		case "calibrated":
			data.calibrated = value >= 1;
			return;
	}

	if (key.startsWith("forward_curve_")) {
		const idx = Number.parseInt(key.slice("forward_curve_".length), 10);
		if (!Number.isNaN(idx)) data.forwardCurve[idx] = value;
		return;
	}

	if (key.startsWith("latent_")) {
		const idx = Number.parseInt(key.slice("latent_".length), 10);
		if (!Number.isNaN(idx)) data.latent[idx] = value;
		return;
	}

	if (!key.startsWith("layer_")) return;

	const parts = key.split("_");
	const layerId = Number.parseInt(parts[1] ?? "", 10);
	const prop = parts[2];
	const idx = Number.parseInt(parts[3] ?? "", 10);

	if (Number.isNaN(layerId)) return;

	if (!data.layers[layerId]) {
		data.layers[layerId] = { state: [], prediction: [] };
	}

	const layer = data.layers[layerId];
	if (prop === "error") {
		layer.errorNorm = value;
		return;
	}

	if (Number.isNaN(idx)) return;

	if (prop === "state") layer.state[idx] = value;
	else if (prop === "prediction") layer.prediction[idx] = value;
};

export const resonanceAt = (
	state: Record<string, Record<string, number>>,
	symbol: string,
): Record<string, number> | undefined =>
	kernelRowAt(state, symbol);

export const resonanceUniverseRows = (
	state: Record<string, Record<string, number>>,
): Array<Record<string, unknown> & { symbol: string }> =>
	Object.entries(state).map(([symbol, row]) => ({ symbol, ...row }));

export const parseResonanceData = (
	row: Record<string, unknown> | null | undefined,
): ResonanceData | undefined => {
	if (!row || typeof row !== "object" || Array.isArray(row)) return undefined;
	if (Object.keys(row).length === 0) return undefined;

	const data: ResonanceData = { forwardCurve: [], latent: [], layers: [] };

	for (const [key, value] of Object.entries(row)) {
		applyKey(data, key, value);
	}

	return finalize(data);
};