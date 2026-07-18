import { describe, expect, it } from "vitest";
import { expandWireMeasurements, splitWireMetricKey } from "./measurement-wire";

describe("expandWireMeasurements", () => {
	it("expands nested metrics into flat readings", () => {
		const readings = expandWireMeasurements([
			{
				source: "sentiment",
				stream: "sentiment",
				symbol: "BTC/USD",
				at: "2026-07-17T09:00:00Z",
				maturity: 0.8,
				validity: { state: "valid", readiness: "observation" },
				metrics: {
					breadth: 1,
					strength: 0.42,
				},
			},
		]);

		expect(readings).toHaveLength(2);
		expect(readings.map((reading) => reading.metric).sort()).toEqual([
			"breadth",
			"strength",
		]);

		const strength = readings.find((reading) => reading.metric === "strength");
		expect(strength?.raw).toBe(0.42);
		expect(strength?.source).toBe("sentiment");
		expect(strength?.symbol).toBe("BTC/USD");
	});

	it("recovers hawkes directional sides from metric:side keys", () => {
		expect(splitWireMetricKey("arrival_rate:buy")).toEqual({
			metric: "arrival_rate",
			side: "buy",
		});

		const readings = expandWireMeasurements([
			{
				source: "hawkes",
				symbol: "BTC/USD",
				at: "2026-07-17T09:00:00Z",
				metrics: {
					"arrival_rate:buy": 1.5,
					"arrival_rate:sell": 0.7,
				},
			},
		]);

		expect(readings).toHaveLength(2);
		expect(readings.find((reading) => reading.side === "buy")?.raw).toBe(1.5);
		expect(readings.find((reading) => reading.side === "sell")?.raw).toBe(0.7);
	});

	it("keeps compact hawkes scale identity for fit reconstruction", () => {
		const fitFrom = "2026-07-14T17:00:00.000Z";
		const fitThrough = "2026-07-14T17:00:10.000Z";
		const evalAt = "2026-07-14T17:00:12.000Z";

		const readings = expandWireMeasurements([
			{
				source: "hawkes",
				symbol: "BTC/USD",
				at: fitThrough,
				scale: {
					kind: "observation_window",
					from: fitFrom,
					through: fitThrough,
				},
				metrics: {
					"baseline_intensity:buy": 0.6,
					"baseline_intensity:sell": 0.4,
					decay_rate: 1,
					spectral_radius: 0.72,
				},
			},
			{
				source: "hawkes",
				symbol: "BTC/USD",
				at: evalAt,
				scale: {
					kind: "observation_window",
					from: fitFrom,
					through: evalAt,
				},
				metrics: {
					"conditional_intensity:buy": 0.9,
					"conditional_intensity:sell": 0.6,
				},
			},
		]);

		const baseline = readings.find(
			(reading) =>
				reading.metric === "baseline_intensity" && reading.side === "buy",
		);
		const intensity = readings.find(
			(reading) =>
				reading.metric === "conditional_intensity" && reading.side === "buy",
		);

		expect(baseline?.scale.from).toBe(fitFrom);
		expect(intensity?.scale.from).toBe(fitFrom);
		expect(intensity?.scale.through).toBe(evalAt);
	});

	it("passes already-flat measurement rows through", () => {
		const readings = expandWireMeasurements([
			{
				source: "leadlag",
				metric: "strength",
				symbol: "BTC/USD",
				at: "2026-07-17T09:00:00Z",
				raw: 0.11,
				normalized: 0.11,
				uncertainty: null,
				validity: { state: "valid", readiness: "observation" },
				scale: {
					kind: "observation_window",
					from: "2026-07-17T09:00:00Z",
					through: "2026-07-17T09:00:00Z",
				},
			},
		]);

		expect(readings).toHaveLength(1);
		expect(readings[0]?.metric).toBe("strength");
		expect(readings[0]?.raw).toBe(0.11);
	});

	it("skips compact and flat rows with missing identity fields", () => {
		expect(
			expandWireMeasurements([
				{
					source: "",
					symbol: "BTC/USD",
					at: "2026-07-17T09:00:00Z",
					metrics: { strength: 0.5 },
				},
				{
					source: "sentiment",
					symbol: "BTC/USD",
					metrics: { strength: 0.5 },
				},
				{
					source: "leadlag",
					metric: "strength",
					symbol: "BTC/USD",
					raw: 0.11,
				},
				{
					source: "leadlag",
					metric: "strength",
					symbol: "BTC/USD",
					at: "2026-07-17T09:00:00Z",
					raw: Number.NaN,
				},
			]),
		).toEqual([]);
	});
});
