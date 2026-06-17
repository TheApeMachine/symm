import {
	type LatentPoint3D,
	parseResonanceXRayFrame,
	type ResonanceLayerFrame,
	type ResonanceXRayFrame,
} from "#/components/charts/resonance/resonance-xray-frame";

export const RESONANCE_LATENT_WIDTH = 3;

export type ResonanceSymbolSummary = {
	symbol: string;
	surprise: number;
	energy: number;
	confidence: number;
	category: string;
	strength: number;
	latent: number[];
};

export type ResonanceUniverseFrame = {
	type: "resonance_universe";
	ts: string;
	arch: number[];
	symbolCount: number;
	focusSymbol: string;
	symbols: ResonanceSymbolSummary[];
	focus: ResonanceXRayFrame;
};

const finiteNumber = (value: unknown): number | null => {
	if (typeof value !== "number" || !Number.isFinite(value)) {
		return null;
	}

	return value;
};

const numberArray = (value: unknown): number[] => {
	if (!Array.isArray(value)) {
		return [];
	}

	const output: number[] = [];

	for (const entry of value) {
		const parsed = finiteNumber(entry);

		if (parsed === null) {
			continue;
		}

		output.push(parsed);
	}

	return output;
};

/*
latentPointFromVector maps the three-mode latent vector into R3.
*/
export const latentPointFromVector = (
	latent: number[],
): LatentPoint3D | null => {
	if (latent.length !== RESONANCE_LATENT_WIDTH) {
		return null;
	}

	return {
		x: latent[0],
		y: latent[1],
		z: latent[2],
	};
};

const parseSymbolSummary = (value: unknown): ResonanceSymbolSummary | null => {
	if (typeof value !== "object" || value === null) {
		return null;
	}

	const row = value as Record<string, unknown>;
	const symbol = typeof row.symbol === "string" ? row.symbol.trim() : "";
	const surprise = finiteNumber(row.surprise);
	const energy = finiteNumber(row.energy);
	const confidence = finiteNumber(row.confidence);
	const strength = finiteNumber(row.strength);
	const category = typeof row.category === "string" ? row.category : "";
	const latent = numberArray(row.latent);

	if (
		symbol === "" ||
		surprise === null ||
		energy === null ||
		confidence === null ||
		strength === null ||
		latent.length !== RESONANCE_LATENT_WIDTH
	) {
		return null;
	}

	return {
		symbol,
		surprise,
		energy,
		confidence,
		category,
		strength,
		latent,
	};
};

/*
parseResonanceUniverseFrame normalizes a batched resonance ui wire frame.
*/
export const parseResonanceUniverseFrame = (
	raw: Record<string, unknown>,
): ResonanceUniverseFrame | null => {
	if (raw.type !== "resonance_universe") {
		return null;
	}

	const focusSymbolRaw =
		typeof raw.focus_symbol === "string" ? raw.focus_symbol.trim() : "";

	if (focusSymbolRaw === "") {
		return null;
	}

	const focusRaw =
		typeof raw.focus === "object" && raw.focus !== null
			? (raw.focus as Record<string, unknown>)
			: null;

	if (focusRaw === null) {
		return null;
	}

	const focus = parseResonanceXRayFrame(focusRaw);

	if (focus === null || focus.symbol !== focusSymbolRaw) {
		return null;
	}

	const symbolsRaw = raw.symbols;

	if (!Array.isArray(symbolsRaw) || symbolsRaw.length === 0) {
		return null;
	}

	const symbols: ResonanceSymbolSummary[] = [];

	for (const symbolRaw of symbolsRaw) {
		const summary = parseSymbolSummary(symbolRaw);

		if (summary === null) {
			return null;
		}

		symbols.push(summary);
	}

	const symbolCount = finiteNumber(raw.symbol_count);

	if (symbolCount === null || symbolCount !== symbols.length) {
		return null;
	}

	const focusPresent = symbols.some(
		(summary) => summary.symbol === focusSymbolRaw,
	);

	if (!focusPresent) {
		return null;
	}

	const ts = typeof raw.ts === "string" ? raw.ts : "";
	const arch = numberArray(raw.arch);

	return {
		type: "resonance_universe",
		ts,
		arch,
		symbolCount,
		focusSymbol: focusSymbolRaw,
		symbols,
		focus,
	};
};

export const sortedUniverseSymbols = (
	symbols: ResonanceSymbolSummary[],
): ResonanceSymbolSummary[] => {
	return [...symbols].sort((left, right) => right.surprise - left.surprise);
};

export const normalizedSurpriseReading = (
	surprise: number,
	surpriseScale: number,
): number => {
	const scale = Math.max(surpriseScale, 1e-6);
	const normalized = Math.log1p(Math.max(surprise, 0)) / Math.log1p(scale);

	return Math.min(1, Math.max(0, normalized));
};

export const categoryFill = (category: string): string => {
	if (category === "turbulent_resonance") {
		return "#ec4899";
	}

	if (category === "laminar_resonance") {
		return "#4ade80";
	}

	return "#38bdf8";
};

export const focusXRayLayers = (
	frame: ResonanceUniverseFrame,
): ResonanceLayerFrame[] => {
	return frame.focus.layers;
};
