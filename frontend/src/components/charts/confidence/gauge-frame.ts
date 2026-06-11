const finiteNumber = (value: unknown): number | null =>
	typeof value === "number" && Number.isFinite(value) ? value : null;

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
