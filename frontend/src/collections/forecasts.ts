import {
	createSnapshotRowStore,
	forecastRowKey,
} from "#/collections/snapshot-retain";
import type { ThesisForecast } from "#/types/thesis";

const asForecasts = (frame: unknown): ThesisForecast[] => {
	if (!Array.isArray(frame)) {
		return [];
	}

	return frame.filter(
		(row): row is ThesisForecast =>
			typeof row === "object" &&
			row !== null &&
			typeof (row as ThesisForecast).symbol === "string" &&
			typeof (row as ThesisForecast).source === "string",
	);
};

/*
forecastsStore merges backend thesis.Forecasts snapshots by row identity so
partial tick frames cannot erase retained symbol evidence.
*/
export const forecastsStore = createSnapshotRowStore(
	"forecasts",
	asForecasts,
	forecastRowKey,
);
