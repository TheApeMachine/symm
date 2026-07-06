import { describe, expect, it } from "vitest";
import { Circular } from "#/collections/circular";
import type { Measurement } from "#/collections/measurements";
import { terminalHealthSummary } from "./health";

describe("terminalHealthSummary", () => {
	it("counts backend measurement frames as firing", () => {
		const history = Circular<Measurement>(4);

		history.push({
			source: "depthflow",
			symbol: "BTC/USD",
			at: "2026-07-06T10:00:00Z",
			status: "measured",
			elapsed: 0,
			entryBaseline: 0.5,
			exitBaseline: 0.25,
			categories: [
				{
					type: "loaded_imbalance",
					confidence: 0.21,
					surprisal: 0,
					strength: 0.4,
				},
			],
			metrics: {},
		});

		const summary = terminalHealthSummary(
			{
				measurements: {
					"BTC/USD": {
						depthflow: history,
					},
				},
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
