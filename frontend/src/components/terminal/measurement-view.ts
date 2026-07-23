import type { Measurement } from "#/types/measurement";
import type { SignalHealthStatus } from "./kernel-meta";

/*
SOURCE_HEADLINE_METRIC selects the native metric that best summarizes sources
whose primary reading is more specific than the shared strength metric.
*/
const SOURCE_HEADLINE_METRIC: Record<string, string | null> = {
	hawkes: "conditional_intensity",
	liquidity: "scarcity_score",
	toxicity: "touch_quantity",
};

/*
SOURCE_HEADLINE_FALLBACKS keeps kernels readable when the preferred metric has
not matured yet. Hawkes publishes event_count/arrival_rate before a fit yields
conditional_intensity; without fallbacks BTC looks OFF FOCUS while live.
*/
const SOURCE_HEADLINE_FALLBACKS: Record<string, readonly string[]> = {
	hawkes: ["conditional_intensity", "arrival_rate", "event_count"],
};

export const headlineMetric = (source: string): string | null =>
	Object.hasOwn(SOURCE_HEADLINE_METRIC, source)
		? SOURCE_HEADLINE_METRIC[source]
		: "strength";

/*
headlineReading returns the newest focus reading for a source, walking each
configured fallback metric before treating the kernel as absent.
*/
export const headlineReading = (
	values: Measurement[],
	source: string,
): Measurement | undefined => {
	const fallbacks = Object.hasOwn(SOURCE_HEADLINE_FALLBACKS, source)
		? SOURCE_HEADLINE_FALLBACKS[source]
		: undefined;

	if (fallbacks !== undefined) {
		for (const metric of fallbacks) {
			const reading = latestByMetric(values, metric);

			if (reading !== undefined) {
				return reading;
			}
		}

		return undefined;
	}

	const headline = headlineMetric(source);

	if (headline === null) {
		return values.at(-1);
	}

	return latestByMetric(values, headline);
};

/*
metricKey is the wire/store lookup for one metric identity, including side when
the buffer row carries a compact metrics map (metric:side).
*/
const metricKey = (metric: string, side = ""): string =>
	side === "" ? metric : `${metric}:${side}`;

/*
readingFromRow returns a flat metric view of one CircularBuffer row. Compact
wire rows store values under metrics; flat rows already carry metric + raw.
*/
const readingFromRow = (
	measurement: Measurement,
	metric: string,
	side = "",
): Measurement | undefined => {
	if (
		measurement.metric === metric &&
		(side === "" || (measurement.side ?? "") === side)
	) {
		return measurement;
	}

	const raw = measurement.metrics?.[metricKey(metric, side)];

	if (typeof raw !== "number" || !Number.isFinite(raw)) {
		return undefined;
	}

	return {
		...measurement,
		metric,
		side: side === "" ? measurement.side : side,
		raw,
		normalized: measurement.normalized ?? null,
	};
};

/*
latestByMetric finds the most recent measurement matching a metric and side.
Side is part of identity because directional metrics (e.g. toxicity's bid/ask
best price) share a metric name.
*/
export const latestByMetric = (
	values: Measurement[],
	metric: string,
	side = "",
): Measurement | undefined => {
	for (let index = values.length - 1; index >= 0; index -= 1) {
		const reading = readingFromRow(values[index], metric, side);

		if (reading !== undefined) {
			return reading;
		}
	}

	return undefined;
};

/*
seriesByMetric returns the raw value history for one metric/side pair, oldest
first, for sparkline rendering.
*/
export const seriesByMetric = (
	values: Measurement[],
	metric: string,
	side = "",
): number[] =>
	values.flatMap((measurement) => {
		const reading = readingFromRow(measurement, metric, side);

		return reading !== undefined && Number.isFinite(reading.raw)
			? [reading.raw]
			: [];
	});

/*
rowsFromBuffer projects each CircularBuffer entry into flat metric rows so
detail grids can iterate metric identity whether the store held a compact
metrics map or already-flat rows.
*/
export const rowsFromBuffer = (values: Measurement[]): Measurement[] =>
	values.flatMap((measurement) => {
		if (measurement.metric !== undefined || measurement.metrics === undefined) {
			return [measurement];
		}

		return Object.entries(measurement.metrics).flatMap(([key, raw]) => {
			if (typeof raw !== "number" || !Number.isFinite(raw)) {
				return [];
			}

			const split = key.indexOf(":");
			const metric = split === -1 ? key : key.slice(0, split);
			const side = split === -1 ? undefined : key.slice(split + 1);

			return [
				{
					...measurement,
					metric,
					side,
					raw,
					normalized: measurement.normalized ?? null,
				},
			];
		});
	});

/*
latestEpoch returns every measurement that shares the most recent observation
time in a flat measurement list.
*/
export const latestEpoch = (values: Measurement[]): Measurement[] => {
	const rows = rowsFromBuffer(values);
	const at = rows.at(-1)?.at;

	if (at === undefined) {
		return [];
	}

	return rows.filter((measurement) => measurement.at === at);
};

/*
measurementIdentity is the stable identity for one backend measurement record.
Metric and side alone are insufficient because cross-section signals can emit
the same metric twice per tick from different subjects or streams.
*/
export const measurementIdentity = (measurement: Measurement): string =>
	JSON.stringify([
		measurement.metric ?? null,
		measurement.side ?? null,
		measurement.subject ?? null,
		measurement.stream ?? null,
	]);

/*
dedupeEpoch keeps the last record for each measurement identity within one
observation tick so duplicate websocket pushes do not create duplicate UI keys.
*/
export const dedupeEpoch = (epoch: Measurement[]): Measurement[] => {
	const byIdentity = new Map<string, Measurement>();

	for (const measurement of epoch) {
		byIdentity.set(measurementIdentity(measurement), measurement);
	}

	return [...byIdentity.values()];
};

/*
orderedEpoch returns the latest observation tick sorted with the headline metric
first so detail surfaces can render every backend metric without arbitrary caps.
*/
export const orderedEpoch = (
	values: Measurement[],
	headline: string | null,
): Measurement[] => {
	const epoch = dedupeEpoch(latestEpoch(values));

	return [...epoch].sort((left, right) => {
		const leftRank = left.metric === headline ? 0 : 1;
		const rightRank = right.metric === headline ? 0 : 1;

		if (leftRank !== rightRank) {
			return leftRank - rightRank;
		}

		const metricCompare = (left.metric ?? "").localeCompare(right.metric ?? "");

		if (metricCompare !== 0) {
			return metricCompare;
		}

		const sideCompare = (left.side ?? "").localeCompare(right.side ?? "");

		if (sideCompare !== 0) {
			return sideCompare;
		}

		const subjectCompare = (left.subject ?? "").localeCompare(
			right.subject ?? "",
		);

		if (subjectCompare !== 0) {
			return subjectCompare;
		}

		return (left.stream ?? "").localeCompare(right.stream ?? "");
	});
};

/*
resolveStatus reads validity and maturity rather than the retired
confidence/surprisal category model. Maturity of exactly zero is treated as
"not reported" rather than "cold start" because several sources never set it.
*/
export const resolveStatus = (
	measurement: Measurement | undefined,
): SignalHealthStatus => {
	if (measurement === undefined) {
		return "waiting";
	}

	if (measurement.validity?.state === "invalid") {
		return "fault";
	}

	if (measurement.validity?.state === "provisional") {
		return "calibrating";
	}

	const maturity = measurement.maturity ?? 0;

	if (maturity > 0 && maturity < 0.5) {
		return "calibrating";
	}

	return "measured";
};

/*
resolveKernelStatus distinguishes a quiet focus symbol from a source the
universe is already measuring elsewhere, so the kernel list does not paint a
STANDBY lie while other symbols are live.
*/
export const resolveKernelStatus = (
	measurement: Measurement | undefined,
	universeHasSource: boolean,
): SignalHealthStatus => {
	if (measurement !== undefined) {
		return resolveStatus(measurement);
	}

	if (universeHasSource) {
		return "unfocused";
	}

	return "waiting";
};

const UNIT_SUFFIX: Record<string, string> = {
	dimensionless: "",
	quote_currency: " quote",
	base_currency: " base",
	quote_currency_per_second: " quote/s",
	base_currency_per_second: " base/s",
	inverse_quote_currency_second: " /(quote·s)",
	count: "",
	events_per_second: "/s",
	inverse_second: "/s",
	second: "s",
	nat: " nat",
};

/*
formatRaw renders one measurement's raw value with a unit-appropriate
precision and suffix, without assuming a currency symbol since quote/base
denomination is pair-specific.
*/
export const formatRaw = (measurement: Measurement): string => {
	const magnitude = Math.abs(measurement.raw);
	const precision = magnitude !== 0 && magnitude < 1 ? 4 : 2;
	const suffix = UNIT_SUFFIX[measurement.unit ?? "dimensionless"] ?? "";

	return `${measurement.raw.toFixed(precision)}${suffix}`;
};

/*
percentOf derives a 0-100 bar fill. Normalized values win when the backend
supplied one; otherwise dimensionless scores already live on roughly a 0-1
scale and clamp directly, while currency and count units have no natural
bound and render without a fill.
*/
export const percentOf = (measurement: Measurement): number => {
	if (typeof measurement.normalized === "number") {
		return Math.max(0, Math.min(100, measurement.normalized * 100));
	}

	if ((measurement.unit ?? "dimensionless") !== "dimensionless") {
		return 0;
	}

	return Math.max(0, Math.min(100, measurement.raw * 100));
};

const METRIC_LABEL_OVERRIDES: Record<string, string> = {
	rvol: "RVOL",
	viscosity: "Replenishment Ratio",
};

/*
metricLabel humanizes a backend snake_case metric key for display.
*/
export const metricLabel = (metric: string | undefined): string => {
	if (!metric) {
		return "Value";
	}

	if (METRIC_LABEL_OVERRIDES[metric]) {
		return METRIC_LABEL_OVERRIDES[metric];
	}

	return metric
		.split("_")
		.map((word) => word.charAt(0).toUpperCase() + word.slice(1))
		.join(" ");
};

const SIDE_LABELS: Record<string, string> = {
	buy: "Bid",
	sell: "Ask",
	buy_to_buy: "Buy→Buy",
	sell_to_buy: "Sell→Buy",
	buy_to_sell: "Buy→Sell",
	sell_to_sell: "Sell→Sell",
};

export const sideLabel = (side: string | undefined): string => {
	if (side === undefined || side === "") {
		return "";
	}

	return SIDE_LABELS[side] ?? side.replaceAll("_", "→");
};

export const stampOf = (at: string | undefined): number =>
	at === undefined ? Number.NaN : Date.parse(at);

export const ageText = (stamp: number): string => {
	if (!Number.isFinite(stamp)) {
		return "—";
	}

	const age = Math.max(0, Date.now() - stamp);

	if (age < 1000) {
		return `${Math.round(age)}ms`;
	}

	return `${(age / 1000).toFixed(1)}s`;
};
