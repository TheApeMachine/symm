const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

const finiteCount = (value: unknown): number =>
	Math.max(0, Math.floor(finiteNumber(value) ?? 0));

const wireString = (frame: Record<string, unknown>, key: string): string => {
	const value = frame[key];

	return typeof value === "string" ? value.trim() : "";
};

const wireBoolean = (frame: Record<string, unknown>, key: string): boolean =>
	frame[key] === true;

const wireRecord = (value: unknown): Record<string, unknown> | null =>
	typeof value === "object" && value !== null
		? (value as Record<string, unknown>)
		: null;

const nestedNumber = (
	frame: Record<string, unknown>,
	...path: string[]
): number | null => {
	let current: unknown = frame;

	for (const segment of path) {
		const record = wireRecord(current);

		if (record === null) {
			return null;
		}

		current = record[segment];
	}

	return finiteNumber(current);
};

/*
normalizeWireFrame maps measurement artifacts and legacy gauge payloads
into the lowercase dashboard wire shape.
*/
export const normalizeWireFrame = (
	frame: Record<string, unknown>,
): Record<string, unknown> => {
	const source =
		wireString(frame, "source") ||
		wireString(frame, "origin") ||
		wireString(frame, "Source");
	const symbol =
		wireString(frame, "symbol") ||
		wireString(frame, "scope") ||
		wireString(frame, "Symbol");
	const confidence =
		nestedNumber(frame, "output", "confidence") ??
		finiteNumber(frame.confidence) ??
		finiteNumber(frame.Confidence);
	const surprise =
		nestedNumber(frame, "cognition", "surprise", "value") ??
		nestedNumber(frame, "output", "surprise") ??
		nestedNumber(frame, "output", "value") ??
		gaugeSurpriseReading(frame) ??
		finiteNumber(frame.Surprise);
	const thresholdReading =
		nestedNumber(frame, "cognition", "surprise", "threshold") ??
		finiteNumber(frame.surprise_threshold) ??
		finiteNumber(frame.surpriseThreshold);
	const samples = finiteCount(frame.samples ?? frame.Samples);
	const minSamples = finiteCount(frame.min_samples ?? frame.minSamples);
	const strength =
		nestedNumber(frame, "output", "strength") ??
		finiteNumber(frame.strength) ??
		finiteNumber(frame.Strength);
	const elapsed =
		nestedNumber(frame, "output", "elapsed") ??
		finiteNumber(frame.elapsed) ??
		finiteNumber(frame.Elapsed);
	const activeReadings = finiteCount(
		frame.active_readings ?? frame.activeReadings ?? frame.ActiveReadings,
	);
	const readingsCapacity = finiteCount(
		frame.readings_capacity ?? frame.readingsCapacity ?? frame.ReadingsCapacity,
	);
	const observedAt =
		frame.observed_at ??
		frame.observedAt ??
		frame.ObservedAt ??
		(typeof frame.timestamp === "number" ? frame.timestamp : undefined);
	const category =
		wireString(frame, "category") ||
		wireString(frame, "Category") ||
		(nestedNumber(frame, "output", "category") !== null
			? String(nestedNumber(frame, "output", "category"))
			: "");
	const gapReason =
		wireString(frame, "gap_reason") ||
		wireString(frame, "gapReason") ||
		wireString(frame, "GapReason");
	const bestEffort =
		wireBoolean(frame, "best_effort") ||
		wireBoolean(frame, "bestEffort") ||
		wireBoolean(frame, "BestEffort");
	const calibrating = frame.calibrating === true || frame.Calibrating === true;
	const calibrated = frame.calibrated === true || frame.Calibrated === true;

	return {
		...frame,
		source: source,
		symbol: symbol,
		confidence: confidence ?? 0,
		surprise: surprise,
		surprise_threshold:
			thresholdReading !== null ? Math.max(0.1, thresholdReading) : 0,
		strength: strength ?? 0,
		elapsed: elapsed ?? 0,
		active_readings: activeReadings,
		readings_capacity: readingsCapacity,
		observed_at: observedAt,
		category: category,
		best_effort: bestEffort,
		gap_reason: gapReason,
		samples: samples,
		min_samples: minSamples,
		calibrating: calibrating,
		calibrated: calibrated,
	};
};

/*
gaugeConfidenceReading mirrors the dashboard gauge needle input.
Confidence is always the softmax category share (1/N for a uniform guess).
*/
export const gaugeConfidenceReading = (
	frame: Record<string, unknown>,
): number | null =>
	nestedNumber(frame, "output", "confidence") ?? finiteNumber(frame.confidence);

/*
gaugeSurpriseReading mirrors the dashboard surprise strip input.
*/
export const gaugeSurpriseReading = (
	frame: Record<string, unknown>,
): number | null => {
	const surpriseReading =
		nestedNumber(frame, "cognition", "surprise", "value") ??
		nestedNumber(frame, "output", "surprise") ??
		nestedNumber(frame, "output", "value") ??
		frame.surprise ??
		frame.snr;

	if (
		typeof surpriseReading !== "number" ||
		!Number.isFinite(surpriseReading)
	) {
		return null;
	}

	return Math.max(0, surpriseReading);
};
