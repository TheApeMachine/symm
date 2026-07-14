import { describe, expect, it } from "vitest";
import { Circular } from "#/collections/circular";
import type { MeasurementEpoch } from "#/collections/measurements";
import { terminalHealthSummary } from "./health";

describe("terminalHealthSummary", () => {
	it("counts backend measurement frames as firing", () => {
		const history = Circular<MeasurementEpoch>(4);

		history.push({
			at: "2026-07-06T10:00:00Z",
			publishedAt: "2026-07-06T10:00:00Z",
			readings: [
				{
					source: "depthflow",
					metric: "strength",
					symbol: "BTC/USD",
					at: "2026-07-06T10:00:00Z",
					raw: 0.21,
					normalized: null,
					uncertainty: null,
					validity: { state: "valid", readiness: "observation" },
					scale: { kind: "", from: "", through: "" },
				},
			],
		});

		const summary = terminalHealthSummary(
			{
				measurements: {
					"BTC/USD": {
						depthflow: history,
					},
				},
				version: 1,
			},
			"BTC/USD",
			["depthflow", "fluid"],
		);

		expect(summary.firing).toBe(1);
		expect(summary.measured).toBe(1);
		expect(summary.total).toBe(2);
		expect(summary.avg).toBe(21);
	});
});
