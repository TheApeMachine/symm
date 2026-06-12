import { bench, describe } from "vitest";

import {
	evidenceMeterValue,
	healthMeterValue,
	parseGaugeFrame,
} from "#/collections/signals";

const evidenceFrame = {
	source: "fluid",
	confidence: 0.64,
	surprise: 2.8,
	surprise_threshold: 2,
	strength: 0.42,
	elapsed: 60,
	active_readings: 12,
	readings_capacity: 16,
	observed_at: new Date().toISOString(),
	calibrated: true,
};

const evidenceReading = parseGaugeFrame(evidenceFrame);

describe("signal diagnostics", () => {
	bench("parse evidence gauge frames", () => {
		parseGaugeFrame(evidenceFrame);
	});

	bench("score signal health", () => {
		if (evidenceReading === null) {
			return;
		}

		healthMeterValue(evidenceReading);
		evidenceMeterValue(evidenceReading);
	});
});
