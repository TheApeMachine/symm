export const confidenceFromGaugePayload = (
	payload: Record<string, unknown>,
): number => {
	const confidence = payload.confidence;

	if (typeof confidence !== "number" || !Number.isFinite(confidence)) {
		return 0;
	}

	return confidence;
};

export const gaugeWarmupPercent = (
	payload: Record<string, unknown>,
): number | null => {
	if (payload.calibrated === true) {
		return null;
	}

	const samples = payload.samples;
	const minSamples = payload.min_samples;

	if (
		typeof minSamples !== "number" ||
		!Number.isFinite(minSamples) ||
		minSamples <= 0
	) {
		return 0;
	}

	const sampleCount =
		typeof samples === "number" && Number.isFinite(samples) ? samples : 0;

	return Math.min(100, (sampleCount / minSamples) * 100);
};

export const formatGaugePayloadValue = (value: unknown): string => {
	if (value === null) {
		return "null";
	}

	if (value === undefined) {
		return "";
	}

	if (typeof value === "number") {
		return Number.isInteger(value) ? value.toString() : value.toFixed(4);
	}

	if (typeof value === "string" || typeof value === "boolean") {
		return String(value);
	}

	return JSON.stringify(value);
};

export const gaugePayloadEntries = (
	payload: Record<string, unknown>,
): ReadonlyArray<readonly [string, string]> => {
	const entries: Array<readonly [string, string]> = [];

	for (const key of Object.keys(payload).sort()) {
		entries.push([key, formatGaugePayloadValue(payload[key])]);
	}

	return entries;
};
