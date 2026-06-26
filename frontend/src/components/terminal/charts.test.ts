import { describe, expect, it } from "vitest";
import { terminalPredictionSampleFromFrame } from "./charts";

describe("terminalPredictionSampleFromFrame", () => {
	it("collapses a resonance layer snapshot into one time-series sample", () => {
		const sample = terminalPredictionSampleFromFrame({
			type: "resonance_universe",
			ts: "2026-06-26T09:55:37Z",
			focus_symbol: "BTC/EUR",
			focus: {
				symbol: "BTC/EUR",
				ts: "2026-06-26T09:55:37Z",
				surprise: 0.12,
				layers: [
					{
						state: [1, 2, 3],
						prediction: [1, 1, 2],
						error_norm: 0.05,
					},
				],
			},
		});

		expect(sample).toMatchObject({
			key: "BTC/EUR:2026-06-26T09:55:37Z",
			symbol: "BTC/EUR",
			error: 0.05,
			actual: 1,
			prediction: 1,
		});
	});

	it("does not fabricate a prediction when the frame has no prediction vector", () => {
		const sample = terminalPredictionSampleFromFrame({
			type: "resonance",
			symbol: "BTC/EUR",
			layers: [{ state: [1, 2, 3], prediction: [] }],
		});

		expect(sample).toBeNull();
	});

	it("selects the requested symbol from universe snapshots instead of backend focus", () => {
		const sample = terminalPredictionSampleFromFrame(
			{
				type: "resonance_universe",
				ts: "2026-06-26T09:55:37Z",
				focus_symbol: "ETH/EUR",
				focus: {
					symbol: "ETH/EUR",
					ts: "2026-06-26T09:55:37Z",
					layers: [{ state: [9], prediction: [8], error_norm: 0.9 }],
				},
				snapshots: [
					{
						symbol: "BTC/EUR",
						ts: "2026-06-26T09:55:37Z",
						layers: [{ state: [3, 4], prediction: [1, 2], error_norm: 0.1 }],
					},
				],
			},
			"BTC/EUR",
		);

		expect(sample).toMatchObject({
			key: "BTC/EUR:2026-06-26T09:55:37Z",
			symbol: "BTC/EUR",
			error: 0.1,
			actual: 3,
			prediction: 1,
		});
	});

	it("ignores universe frames that do not contain the requested symbol", () => {
		const sample = terminalPredictionSampleFromFrame(
			{
				type: "resonance_universe",
				ts: "2026-06-26T09:55:37Z",
				focus_symbol: "ETH/EUR",
				focus: {
					symbol: "ETH/EUR",
					ts: "2026-06-26T09:55:37Z",
					layers: [{ state: [9], prediction: [8], error_norm: 0.9 }],
				},
				snapshots: [],
			},
			"BTC/EUR",
		);

		expect(sample).toBeNull();
	});
});
