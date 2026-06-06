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

export const parsePredictionWire = (
	raw: Record<string, unknown>,
): PredictionReading | null => {
	if (raw.chart !== "prediction") {
		return null;
	}

	if (!isPredictionSeriesKind(raw.kind)) {
		return null;
	}

	if (!isFiniteNumber(raw.x) || !isFiniteNumber(raw.value)) {
		return null;
	}

	return { kind: raw.kind, x: raw.x, value: raw.value };
};

export const deliverPredictionWire = (
	bridge: PredictionBridge,
	reading: PredictionReading,
) => {
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

const isFiniteNumber = (value: unknown): value is number => {
	return typeof value === "number" && Number.isFinite(value);
};
