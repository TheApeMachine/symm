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

/*
normalizeWireFrame maps bulk story measurements and legacy gauge payloads
into the lowercase dashboard wire shape.
*/
export const normalizeWireFrame = (
	frame: Record<string, unknown>,
): Record<string, unknown> => {
	const source = wireString(frame, "source") || wireString(frame, "Source");
	const confidence =
		finiteNumber(frame.confidence) ?? finiteNumber(frame.Confidence);
	const surprise =
		gaugeSurpriseReading(frame) ?? finiteNumber(frame.Surprise) ?? 0;
	const thresholdReading =
		finiteNumber(frame.surprise_threshold) ??
		finiteNumber(frame.surpriseThreshold);
	const samples = finiteCount(frame.samples ?? frame.Samples);
	const minSamples = finiteCount(frame.min_samples ?? frame.minSamples);
	const strength = finiteNumber(frame.strength) ?? finiteNumber(frame.Strength);
	const elapsed = finiteNumber(frame.elapsed) ?? finiteNumber(frame.Elapsed);
	const activeReadings = finiteCount(
		frame.active_readings ?? frame.activeReadings ?? frame.ActiveReadings,
	);
	const readingsCapacity = finiteCount(
		frame.readings_capacity ?? frame.readingsCapacity ?? frame.ReadingsCapacity,
	);
	const observedAt = frame.observed_at ?? frame.observedAt ?? frame.ObservedAt;
	const category =
		wireString(frame, "category") || wireString(frame, "Category");
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
		confidence: confidence ?? 0,
		surprise: surprise,
		surprise_threshold:
			thresholdReading !== null ? Math.max(0.1, thresholdReading) : 2,
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
): number | null => finiteNumber(frame.confidence);

/*
gaugeSurpriseReading mirrors the dashboard surprise strip input.
*/
export const gaugeSurpriseReading = (
	frame: Record<string, unknown>,
): number | null => {
	const surpriseReading = frame.surprise ?? frame.snr;

	if (
		typeof surpriseReading !== "number" ||
		!Number.isFinite(surpriseReading)
	) {
		return null;
	}

	return Math.max(0, surpriseReading);
};
