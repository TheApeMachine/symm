import type {
	Measurement,
	MeasurementValidity,
	ScaleReference,
} from "#/types/measurement";

type WireRow = {
	source?: string;
	stream?: string;
	symbol?: string;
	at?: string;
	maturity?: number;
	validity?: MeasurementValidity;
	metrics?: Record<string, number>;
	metric?: string;
	raw?: number;
	side?: string;
	normalized?: number | null;
	uncertainty?: Measurement["uncertainty"];
	scale?: ScaleReference;
	status?: string;
	elapsed?: number;
	entryBaseline?: number;
	exitBaseline?: number;
	categories?: Measurement["categories"];
};

const isRecord = (value: unknown): value is Record<string, unknown> =>
	typeof value === "object" && value !== null && !Array.isArray(value);

const isNonEmptyString = (value: unknown): value is string =>
	typeof value === "string" && value.trim() !== "";

/*
splitWireMetricKey recovers metric and optional side from a wire metrics key.
Directional Hawkes values publish as metric:side.
*/
export const splitWireMetricKey = (
	key: string,
): { metric: string; side: string } => {
	const separator = key.indexOf(":");

	if (separator < 0) {
		return { metric: key, side: "" };
	}

	return {
		metric: key.slice(0, separator),
		side: key.slice(separator + 1),
	};
};

/*
expandWireMeasurements turns compact source×symbol wire maps with nested
metrics into the flat Measurement rows the store and kernels already consume.
Already-flat rows pass through unchanged.
*/
export const expandWireMeasurements = (frames: unknown): Measurement[] => {
	if (!Array.isArray(frames)) {
		return [];
	}

	const readings: Measurement[] = [];

	for (const frame of frames) {
		if (!isRecord(frame)) {
			continue;
		}

		const row = frame as WireRow;
		const metrics = row.metrics;

		if (metrics !== undefined && isRecord(metrics)) {
			if (
				!isNonEmptyString(row.source) ||
				!isNonEmptyString(row.symbol) ||
				!isNonEmptyString(row.at)
			) {
				continue;
			}

			const source = row.source;
			const symbol = row.symbol;
			const at = row.at;
			const validity = row.validity ?? {
				state: "valid",
				readiness: "observation",
			};
			const scale = row.scale ?? {
				kind: "observation_window",
				from: at,
				through: at,
			};

			for (const [key, raw] of Object.entries(metrics)) {
				if (typeof raw !== "number" || !Number.isFinite(raw)) {
					continue;
				}

				const { metric, side } = splitWireMetricKey(key);

				if (metric === "") {
					continue;
				}

				readings.push({
					source,
					stream: row.stream,
					symbol,
					at,
					metric,
					side: side === "" ? undefined : side,
					raw,
					normalized: null,
					maturity: row.maturity,
					uncertainty: row.uncertainty ?? null,
					validity,
					scale,
					status: row.status,
					elapsed: row.elapsed,
					entryBaseline: row.entryBaseline,
					exitBaseline: row.exitBaseline,
					categories: row.categories,
					metrics,
				});
			}

			continue;
		}

		if (
			isNonEmptyString(row.source) &&
			isNonEmptyString(row.symbol) &&
			isNonEmptyString(row.at) &&
			typeof row.raw === "number" &&
			Number.isFinite(row.raw)
		) {
			readings.push({
				source: row.source,
				stream: row.stream,
				symbol: row.symbol,
				at: row.at,
				metric: row.metric,
				side: row.side,
				raw: row.raw,
				normalized: row.normalized ?? null,
				maturity: row.maturity,
				uncertainty: row.uncertainty ?? null,
				validity: row.validity ?? {
					state: "valid",
					readiness: "observation",
				},
				scale: row.scale ?? {
					kind: "observation_window",
					from: row.at,
					through: row.at,
				},
				status: row.status,
				elapsed: row.elapsed,
				entryBaseline: row.entryBaseline,
				exitBaseline: row.exitBaseline,
				categories: row.categories,
				metrics: row.metrics,
			});
		}
	}

	return readings;
};
