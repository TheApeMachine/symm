const gaugeWireSkipKeys = new Set(["chart", "event"]);

export const gaugeWirePayload = (
	raw: Record<string, unknown>,
): Record<string, unknown> => {
	const payload: Record<string, unknown> = {};

	for (const [key, value] of Object.entries(raw)) {
		if (gaugeWireSkipKeys.has(key)) {
			continue;
		}

		payload[key] = value;
	}

	return payload;
};

export const confidenceFromGaugePayload = (
	payload: Record<string, unknown>,
): number => {
	const confidence = payload.confidence;

	if (typeof confidence !== "number" || !Number.isFinite(confidence)) {
		return 0;
	}

	return confidence;
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
