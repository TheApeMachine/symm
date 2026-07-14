import type { Measurement } from "#/types/measurement";
import type { SignalHealthStatus } from "./kernel-meta";

/*
Sources that reduce their metrics to one composite score expose it as
"strength" (types.MetricStrength on the backend). Toxicity has no composite
score — it reports raw touch, price, and fill quantities — so it has no
headline metric and is rendered as a stat strip instead of a score bar.
*/
const SOURCE_HEADLINE_METRIC: Record<string, string | null> = {
	toxicity: null,
};

export const headlineMetric = (source: string): string | null =>
	source in SOURCE_HEADLINE_METRIC
		? SOURCE_HEADLINE_METRIC[source]
		: "strength";

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
		const measurement = values[index];

		if (measurement.metric === metric && (measurement.side ?? "") === side) {
			return measurement;
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
		if (measurement.metric !== metric || (measurement.side ?? "") !== side) {
			return [];
		}

		return Number.isFinite(measurement.raw) ? [measurement.raw] : [];
	});

/*
latestEpoch returns every measurement that shares the most recent observation
time in the buffer — one signal tick's complete readout.
*/
export const latestEpoch = (values: Measurement[]): Measurement[] => {
	const at = values.at(-1)?.at;

	if (at === undefined) {
		return [];
	}

	return values.filter((measurement) => measurement.at === at);
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

const UNIT_SUFFIX: Record<string, string> = {
	dimensionless: "",
	quote_currency: " quote",
	base_currency: " base",
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

export const sideLabel = (side: string | undefined): string => {
	if (side === "buy") return "Bid";
	if (side === "sell") return "Ask";

	return "";
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
