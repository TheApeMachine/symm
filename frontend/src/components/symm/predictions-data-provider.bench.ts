import { bench, describe } from "vitest";

import { PredictionsDataProvider } from "#/components/symm/predictions-data-provider";

const firstPulseAt = "2026-05-28T01:30:00.000Z";
const secondPulseAt = "2026-05-28T01:30:10.000Z";

describe("PredictionsDataProvider", () => {
	bench("routes aggregate pulse prediction and error readings", () => {
		PredictionsDataProvider.reset();
		PredictionsDataProvider.registerSink(() => undefined);
		PredictionsDataProvider.ingest({
			event: "engine_pulse",
			seq: 1,
			phase: "scan",
			measurements: 8,
			open: 2,
			ts: firstPulseAt,
			avg_prediction: 0.005,
			avg_error: 0.0038,
		});
		PredictionsDataProvider.ingest({
			event: "engine_pulse",
			seq: 2,
			phase: "scan",
			measurements: 10,
			open: 2,
			ts: secondPulseAt,
			avg_prediction: 0.0054,
			avg_error: 0.004,
		});
	});
});
