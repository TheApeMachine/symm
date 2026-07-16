import { createStore } from "@tanstack/react-store";
import type { ThesisForecast } from "#/types/thesis";
import { Circular, type CircularBuffer } from "./circular";

const FORECAST_HISTORY_LIMIT = 50;

const forecastKey = (row: ThesisForecast): string =>
	`${row.symbol}:${row.source}:${row.target}:${row.sourceEpoch}`;

const cloneForecastBuffer = (
	buffer: CircularBuffer<ThesisForecast>,
): CircularBuffer<ThesisForecast> => {
	const cloned = Circular<ThesisForecast>(buffer.capacity());

	for (const value of buffer.values()) {
		cloned.push(value);
	}

	return cloned;
};

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
forecastValues returns the newest retained forecast row for each identity.
*/
export const forecastValues = (
	forecasts: Record<string, CircularBuffer<ThesisForecast>>,
): ThesisForecast[] =>
	Object.keys(forecasts)
		.sort()
		.flatMap((key) => {
			const forecast = forecasts[key]?.values().at(-1);

			return forecast === undefined ? [] : [forecast];
		});

/*
forecastsStore retains backend thesis forecasts in bounded circular buffers so
partial tick frames cannot erase retained symbol evidence.
*/
export const forecastsStore = createStore(
	{
		forecasts: {} as Record<string, CircularBuffer<ThesisForecast>>,
		version: 0,
	},
	({ setState }) => ({
		updateFrame: (frame: unknown) =>
			setState((prev) => {
				const rows = asForecasts(frame);

				if (rows.length === 0) {
					return prev;
				}

				const forecasts = { ...prev.forecasts };

				for (const row of rows) {
					const key = forecastKey(row);
					const existing = forecasts[key];

					if (existing === undefined) {
						const buffer = Circular<ThesisForecast>(FORECAST_HISTORY_LIMIT);
						buffer.push(row);
						forecasts[key] = buffer;
						continue;
					}

					const buffer = cloneForecastBuffer(existing);
					buffer.push(row);
					forecasts[key] = buffer;
				}

				return {
					forecasts,
					version: prev.version + 1,
				};
			}),
		reset: () =>
			setState(() => ({
				forecasts: {},
				version: 0,
			})),
	}),
);
