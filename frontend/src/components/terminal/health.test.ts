import { describe, expect, it } from "vitest";
import type { Measurement } from "#/collections/types";
import { terminalHealthSummary } from "./health";
import { regimeAxes } from "./regime-radar";

const reading = (
	source: string,
	symbol: string,
	metric: string,
	raw: number,
	normalized: number | null = null,
): Measurement => ({
	source,
	metric,
	symbol,
	at: "2026-07-06T10:00:00Z",
	raw,
	normalized,
	uncertainty: null,
	validity: { state: "valid", readiness: "observation" },
	scale: { kind: "observation_window", from: "", through: "" },
});

describe("terminalHealthSummary", () => {
	it("counts backend measurement frames as firing", () => {
		const summary = terminalHealthSummary(
			[reading("depthflow", "BTC/USD", "strength", 0.21)],
			"BTC/USD",
			["depthflow", "hawkes"],
			{ count: 12, completed: true, ns: 45_000_000 },
		);

		expect(summary.firing).toBe(1);
		expect(summary.measured).toBe(1);
		expect(summary.total).toBe(2);
		expect(summary.avg).toBe(21);
		expect(summary.label).toBe("Live");
		expect(summary.tickMs).toBe(45);
		expect(summary.completed).toBe(true);
	});

	it("honors explicit completed false over tick count", () => {
		const summary = terminalHealthSummary(
			[],
			"BTC/USD",
			["depthflow"],
			{ count: 12, completed: false, ns: 12_000_000 },
		);

		expect(summary.completed).toBe(false);
		expect(summary.label).toBe("Silent");
	});

	it("does not call a silent focus STANDBY when the universe is live", () => {
		const summary = terminalHealthSummary(
			[reading("depthflow", "ETH/USD", "strength", 0.21)],
			"BTC/USD",
			["depthflow"],
			{ count: 3, completed: true, ns: 12_000_000 },
		);

		expect(summary.firing).toBe(1);
		expect(summary.label).toBe("Live · thin focus");
		expect(summary.completed).toBe(true);
	});

	it("projects numerical measurements onto the backend regime axes", () => {
		expect(
			regimeAxes(
				[
					reading("hawkes", "BTC/USD", "spectral_radius", 0.8, 0.8),
					reading("pumpdump", "BTC/USD", "trend", 0.6, 0.6),
					reading("cvd", "BTC/USD", "net", -12),
					reading("cvd", "BTC/USD", "net_fraction", 0.7, 0.7),
					reading("cvd", "BTC/USD", "balance", 0.3, 0.3),
				],
				["hawkes", "pumpdump", "cvd"],
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
