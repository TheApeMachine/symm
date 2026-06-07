import type { PredictionBridge } from "#/components/charts/prediction/PredictionChart";
import type {
	PredictionReading,
	PredictionSeriesKind,
} from "#/components/charts/prediction/predictions-data-provider";

const predictionSeriesKinds = new Set<PredictionSeriesKind>([
	"actual",
	"prediction",
	"error",
]);

export const isPredictionWire = (
	raw: unknown,
): raw is PredictionReading & {
	chart: "prediction";
} => {
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

export const ingestPredictionWire = (
	bridge: PredictionBridge | null | undefined,
	raw: unknown,
): void => {
	if (!bridge || !isPredictionWire(raw)) {
		return;
	}

	const reading: PredictionReading = {
		kind: raw.kind,
		x: raw.x,
		value: raw.value,
	};

	if (bridge.ready) {
		bridge.append(reading);
		return;
	}

	bridge.pending.push(reading);
};

const isPredictionSeriesKind = (
	value: unknown,
): value is PredictionSeriesKind => {
	if (typeof value !== "string") {
		return false;
	}

	return predictionSeriesKinds.has(value as PredictionSeriesKind);
};
