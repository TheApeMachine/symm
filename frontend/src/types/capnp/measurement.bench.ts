import { bench, describe } from "vitest";
import { readWireMeasurement } from "./measurement";
import { measurementBytes } from "./measurement.fixture";

describe("readWireMeasurement", () => {
	bench("decodes the Go measurement with metrics and provenance", () => {
		readWireMeasurement(measurementBytes);
	});
});
