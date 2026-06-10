export type PredictionPoint = {
	kind: "actual" | "prediction" | "error";
	x: number;
	value: number;
	horizon?: number;
};

export type PredictionWire = PredictionPoint & {
	chart: "prediction";
};

type PointSink = (point: PredictionPoint) => void;

let chartSink: PointSink | null = null;
const pendingPoints: PredictionPoint[] = [];

const predictionKinds = new Set<PredictionPoint["kind"]>([
	"actual",
	"prediction",
	"error",
]);

export const isPredictionWire = (raw: unknown): raw is PredictionWire => {
	if (typeof raw !== "object" || raw === null) {
		return false;
	}

	const row = raw as Record<string, unknown>;

	if (row.chart !== "prediction") {
		return false;
	}

	if (
		typeof row.kind !== "string" ||
		!predictionKinds.has(row.kind as PredictionPoint["kind"])
	) {
		return false;
	}

	return (
		typeof row.x === "number" &&
		Number.isFinite(row.x) &&
		typeof row.value === "number" &&
		Number.isFinite(row.value)
	);
};

const pointFromWire = (wire: PredictionWire): PredictionPoint => {
	const point: PredictionPoint = {
		kind: wire.kind,
		x: wire.x,
		value: wire.value,
	};

	if (
		typeof wire.horizon === "number" &&
		Number.isFinite(wire.horizon) &&
		wire.horizon > 0
	) {
		point.horizon = wire.horizon;
	}

	return point;
};

const flushPending = (sink: PointSink): void => {
	for (const point of pendingPoints) {
		sink(point);
	}

	pendingPoints.length = 0;
};

export const registerPredictionChart = (
	appendPoint: PointSink,
): (() => void) => {
	chartSink = appendPoint;
	flushPending(appendPoint);

	return () => {
		if (chartSink === appendPoint) {
			chartSink = null;
		}
	};
};

export const ingestPredictionWire = (raw: unknown): void => {
	if (!isPredictionWire(raw)) {
		return;
	}

	const point = pointFromWire(raw);

	if (chartSink) {
		chartSink(point);
		return;
	}

	pendingPoints.push(point);

	if (pendingPoints.length > 400) {
		pendingPoints.splice(0, pendingPoints.length - 400);
	}
};
