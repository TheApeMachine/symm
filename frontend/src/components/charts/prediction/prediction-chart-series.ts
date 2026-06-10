import type { XyDataSeries } from "scichart";

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

export {
	PREDICTION_VALUE_MAX,
	PREDICTION_VALUE_MIN,
	predictionVisibleXRange,
	predictionWindowMin,
} from "#/components/charts/prediction/prediction-chart-state";

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
