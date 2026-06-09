export const confidenceFromGaugePayload = (
	payload: Record<string, unknown>,
): number => {
	const confidence = payload.confidence;

	if (typeof confidence !== "number" || !Number.isFinite(confidence)) {
		return 0;
	}

	return confidence;
};

export const surpriseFromGaugePayload = (
	payload: Record<string, unknown>,
): number => {
	const surprise = payload.surprise ?? payload.snr;

	if (typeof surprise !== "number" || !Number.isFinite(surprise)) {
		return 0;
	}

	return Math.max(0, surprise);
};

export const surpriseThresholdFromGaugePayload = (
	payload: Record<string, unknown>,
): number | null => {
	const threshold = payload.surprise_threshold;

	if (typeof threshold !== "number" || !Number.isFinite(threshold)) {
		return null;
	}

	return Math.max(0.1, threshold);
};

export const surpriseScaleMax = (
	payload: Record<string, unknown>,
	_surprise: number,
): number => {
	const threshold = surpriseThresholdFromGaugePayload(payload) ?? 2;

	return threshold * 3;
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
