import { describe, expect, it } from "vitest";
import { Circular } from "#/collections/circular";
import type { MeasurementEpoch } from "#/collections/measurements";
import { regimeAxes, terminalHealthSummary } from "./health";

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

	it("projects numerical measurements onto the backend regime axes", () => {
		const fluid = Circular<MeasurementEpoch>(4);
		const pumpdump = Circular<MeasurementEpoch>(4);
		const cvd = Circular<MeasurementEpoch>(4);
		const validity = { state: "valid", readiness: "observation" };
		const scale = { kind: "observation_window", from: "", through: "" };

		fluid.push({
			at: "2026-07-15T09:00:00Z",
			publishedAt: "2026-07-15T09:00:00Z",
			readings: [
				{
					source: "fluid",
					metric: "turbulent_score",
					symbol: "BTC/USD",
					at: "2026-07-15T09:00:00Z",
					raw: 0.8,
					normalized: 0.8,
					uncertainty: null,
					validity,
					scale,
				},
			],
		});
		pumpdump.push({
			at: "2026-07-15T09:00:00Z",
			publishedAt: "2026-07-15T09:00:00Z",
			readings: [
				{
					source: "pumpdump",
					metric: "trend",
					symbol: "BTC/USD",
					at: "2026-07-15T09:00:00Z",
					raw: 0.6,
					normalized: 0.6,
					uncertainty: null,
					validity,
					scale,
				},
			],
		});
		cvd.push({
			at: "2026-07-15T09:00:00Z",
			publishedAt: "2026-07-15T09:00:00Z",
			readings: [
				{
					source: "cvd",
					metric: "net",
					symbol: "BTC/USD",
					at: "2026-07-15T09:00:00Z",
					raw: -12,
					normalized: null,
					uncertainty: null,
					validity,
					scale,
				},
				{
					source: "cvd",
					metric: "net_fraction",
					symbol: "BTC/USD",
					at: "2026-07-15T09:00:00Z",
					raw: 0.7,
					normalized: 0.7,
					uncertainty: null,
					validity,
					scale,
				},
				{
					source: "cvd",
					metric: "balance",
					symbol: "BTC/USD",
					at: "2026-07-15T09:00:00Z",
					raw: 0.3,
					normalized: 0.3,
					uncertainty: null,
					validity,
					scale,
				},
			],
		});

		expect(
			regimeAxes(
				{
					measurements: { "BTC/USD": { fluid, pumpdump, cvd } },
					version: 1,
				},
				["fluid", "pumpdump", "cvd"],
			),
		).toEqual([
			{ label: "volatility", value: 0.8 },
			{ label: "trend", value: 0.6 },
			{ label: "bullish", value: 0 },
			{ label: "bearish", value: 0.7 },
			{ label: "chop", value: 0.3 },
		]);
	});
});
