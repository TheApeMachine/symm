import { describe, expect, it } from "vitest";
import type { Measurement } from "#/collections/types";
import { terminalHealthSummary } from "./health";

const reading = (
	source: string,
	symbol: string,
	metric: string,
	raw: number,
	normalized: number | null = null,
): Measurement => ({
	source,
	symbol,
	at: "2026-07-06T10:00:00Z",
	metrics: {
		[metric]: { raw, normalized: normalized ?? raw },
	},
	uncertainty: null,
});

describe("terminalHealthSummary", () => {
	it("counts backend measurement frames as firing", () => {
		const summary = terminalHealthSummary(
			[reading("depthflow", "BTC/USD", "hypothesis_separation", 0.21)],
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
		const summary = terminalHealthSummary([], "BTC/USD", ["depthflow"], {
			count: 12,
			completed: false,
			ns: 12_000_000,
		});

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
});
