export const PREDICTION_VALUE_MIN = -1;
export const PREDICTION_VALUE_MAX = 1;

export const predictionWindowMin = (
	rightEdge: number,
	horizonSec: number,
): number => rightEdge - 2 * horizonSec;

export const predictionVisibleXRange = (
	horizonSec: number,
	forecastTipX: number | null,
	actualEarliestX: number | null,
	errorEarliestX: number | null,
	nowSec: number,
): { minX: number; maxX: number } => {
	const maxX = forecastTipX ?? nowSec + horizonSec;
	let minX = predictionWindowMin(maxX, horizonSec);

	const groundMin = Math.min(
		actualEarliestX ?? Number.POSITIVE_INFINITY,
		errorEarliestX ?? Number.POSITIVE_INFINITY,
	);

	if (groundMin < minX) {
		minX = groundMin;
	}

	const maxSpan = 4 * horizonSec;

	if (maxX - minX > maxSpan) {
		minX = maxX - maxSpan;
	}

	return { minX, maxX };
};
