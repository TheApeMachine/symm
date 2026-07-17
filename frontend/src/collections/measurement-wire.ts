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
			const at = String(row.at ?? "");
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

				readings.push({
					source: String(row.source ?? ""),
					stream: row.stream,
					symbol: String(row.symbol ?? ""),
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

		if (typeof row.symbol === "string" && typeof row.raw === "number") {
			readings.push(row as Measurement);
		}
	}

	return readings;
};
