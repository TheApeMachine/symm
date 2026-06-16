export type ResonanceLayerFrame = {
	state: number[];
	prediction: number[];
	errorNorm: number;
};

export type ResonanceXRayFrame = {
	symbol: string;
	category: string;
	surprise: number;
	energy: number;
	confidence: number;
	layers: ResonanceLayerFrame[];
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

const parseLayer = (value: unknown): ResonanceLayerFrame | null => {
	if (typeof value !== "object" || value === null) {
		return null;
	}

	const layer = value as Record<string, unknown>;

	return {
		state: numberArray(layer.state),
		prediction: numberArray(layer.prediction),
		errorNorm: finiteNumber(layer.error_norm) ?? 0,
	};
};

/*
parseResonanceXRayFrame normalizes a resonance ui wire frame for chart ingestion.
*/
export const parseResonanceXRayFrame = (
	raw: Record<string, unknown>,
): ResonanceXRayFrame | null => {
	const symbol = typeof raw.symbol === "string" ? raw.symbol.trim() : "";

	if (symbol === "") {
		return null;
	}

	const layersRaw = raw.layers;

	if (!Array.isArray(layersRaw) || layersRaw.length === 0) {
		return null;
	}

	const layers: ResonanceLayerFrame[] = [];

	for (const layerRaw of layersRaw) {
		const layer = parseLayer(layerRaw);

		if (layer === null || layer.state.length === 0) {
			continue;
		}

		layers.push(layer);
	}

	if (layers.length === 0) {
		return null;
	}

	const surprise = finiteNumber(raw.surprise) ?? 0;
	const energy = finiteNumber(raw.energy) ?? 0;
	const confidence = finiteNumber(raw.confidence) ?? 0;
	const category = typeof raw.category === "string" ? raw.category : "";

	return {
		symbol,
		category,
		surprise,
		energy,
		confidence,
		layers,
	};
};

export const flattenResonanceStates = (
	layers: ResonanceLayerFrame[],
): number[] => {
	const flattened: number[] = [];

	for (const layer of layers) {
		for (const value of layer.state) {
			flattened.push(value);
		}
	}

	return flattened;
};

export const resonanceChannelLabels = (
	layers: ResonanceLayerFrame[],
): string[] => {
	const inputLabels = ["price", "spread", "volume", "chg"];
	const labels: string[] = [];

	for (let layerIndex = 0; layerIndex < layers.length; layerIndex += 1) {
		const layer = layers[layerIndex];

		for (
			let neuronIndex = 0;
			neuronIndex < layer.state.length;
			neuronIndex += 1
		) {
			if (layerIndex === 0 && neuronIndex < inputLabels.length) {
				labels.push(inputLabels[neuronIndex] ?? `in${neuronIndex}`);
				continue;
			}

			if (layerIndex === layers.length - 1) {
				labels.push(`z${neuronIndex}`);
				continue;
			}

			labels.push(`h${neuronIndex}`);
		}
	}

	return labels;
};

export const shiftHeatmapRow = (
	row: number[],
	value: number,
	columnCount: number,
) => {
	for (let columnIndex = 0; columnIndex < columnCount - 1; columnIndex += 1) {
		row[columnIndex] = row[columnIndex + 1];
	}

	row[columnCount - 1] = value;
};

export type LatentPoint3D = {
	x: number;
	y: number;
	z: number;
};

/*
latentTopLayer returns the settled top latent layer from a resonance frame.
*/
export const latentTopLayer = (
	layers: ResonanceLayerFrame[],
): ResonanceLayerFrame | undefined => {
	return layers.at(-1);
};

/*
latentPoint3D maps the top latent state into R3 for phase-portrait rendering.
*/
export const latentPoint3D = (layers: ResonanceLayerFrame[]): LatentPoint3D => {
	const topLayer = latentTopLayer(layers);

	if (topLayer === undefined || topLayer.state.length === 0) {
		return { x: 0, y: 0, z: 0 };
	}

	const state = topLayer.state;

	if (state.length === 1) {
		return { x: state[0], y: 0, z: 0 };
	}

	if (state.length === 2) {
		return { x: state[0], y: 0, z: state[1] };
	}

	return {
		x: state[0],
		y: state[1],
		z: state[2],
	};
};
