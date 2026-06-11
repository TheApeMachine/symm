import type { XyDataSeries } from "scichart";

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
) => {
	if (horizonSec <= 0) {
		throw new RangeError(
			"predictionVisibleXRange: horizonSec must be positive",
		);
	}

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

export const upsertSortedPoint = (
	dataSeries: XyDataSeries,
	x: number,
	value: number,
): void => {
	const nativeX = dataSeries.getNativeXValues();
	const count = dataSeries.count();

	for (let index = 0; index < count; index += 1) {
		const existingX = nativeX.get(index);

		if (existingX === x) {
			dataSeries.update(index, value);
			return;
		}

		if (existingX > x) {
			dataSeries.insert(index, x, value);
			return;
		}
	}

	dataSeries.append(x, value);
};

export const pruneSeriesBefore = (
	dataSeries: XyDataSeries,
	minX: number,
): void => {
	const nativeX = dataSeries.getNativeXValues();
	const nativeY = dataSeries.getNativeYValues();
	const nextX: number[] = [];
	const nextY: number[] = [];

	for (let index = 0; index < dataSeries.count(); index += 1) {
		const x = nativeX.get(index);

		if (x < minX) {
			continue;
		}

		nextX.push(x);
		nextY.push(nativeY.get(index));
	}

	dataSeries.clear();

	for (let index = 0; index < nextX.length; index += 1) {
		dataSeries.append(nextX[index] ?? 0, nextY[index] ?? 0);
	}
};

export const seriesLatestX = (dataSeries: XyDataSeries): number | null => {
	const count = dataSeries.count();

	if (count <= 0) {
		return null;
	}

	return dataSeries.getNativeXValues().get(count - 1);
};

export const seriesEarliestX = (dataSeries: XyDataSeries): number | null => {
	const count = dataSeries.count();

	if (count <= 0) {
		return null;
	}

	return dataSeries.getNativeXValues().get(0);
};
