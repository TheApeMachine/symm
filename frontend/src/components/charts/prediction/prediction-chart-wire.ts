export type PredictionSeriesKind = "actual" | "prediction" | "error";

export type PredictionReading = {
	kind: PredictionSeriesKind;
	x: number;
	value: number;
	horizon?: number;
};

export type PredictionWire = PredictionReading & {
	chart: "prediction";
};

type ReadingSink = (reading: PredictionReading) => void;

let chartSink: ReadingSink | null = null;
const pendingReadings: PredictionReading[] = [];

const predictionSeriesKinds = new Set<PredictionSeriesKind>([
	"actual",
	"prediction",
	"error",
]);

const isPredictionSeriesKind = (
	value: unknown,
): value is PredictionSeriesKind => {
	if (typeof value !== "string") {
		return false;
	}

	return predictionSeriesKinds.has(value as PredictionSeriesKind);
};

export const isPredictionWire = (raw: unknown): raw is PredictionWire => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	if (row.chart !== "prediction") {
		return false;
	}

	if (!isPredictionSeriesKind(row.kind)) {
		return false;
	}

	return (
		typeof row.x === "number" &&
		Number.isFinite(row.x) &&
		typeof row.value === "number" &&
		Number.isFinite(row.value)
	);
};

const readingFromWire = (wire: PredictionWire): PredictionReading => {
	const reading: PredictionReading = {
		kind: wire.kind,
		x: wire.x,
		value: wire.value,
	};

	if (
		typeof wire.horizon === "number" &&
		Number.isFinite(wire.horizon) &&
		wire.horizon > 0
	) {
		reading.horizon = wire.horizon;
	}

	return reading;
};

const flushPending = (sink: ReadingSink): void => {
	for (const reading of pendingReadings) {
		sink(reading);
	}

	pendingReadings.length = 0;
};

/*
registerPredictionChart connects a mounted chart appendReading to prediction frames.
ingestPredictionWire routes backend chart rows to the registered sink only.
*/
export const registerPredictionChart = (
	appendReading: ReadingSink,
): (() => void) => {
	chartSink = appendReading;
	flushPending(appendReading);

	return () => {
		if (chartSink === appendReading) {
			chartSink = null;
		}
	};
};

export const ingestPredictionWire = (raw: unknown): void => {
	if (!isPredictionWire(raw)) {
		return;
	}

	const reading = readingFromWire(raw);

	if (chartSink) {
		chartSink(reading);
		return;
	}

	pendingReadings.push(reading);

	if (pendingReadings.length > 400) {
		pendingReadings.splice(0, pendingReadings.length - 400);
	}
};
