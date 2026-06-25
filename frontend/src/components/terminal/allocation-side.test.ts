import { describe, expect, it } from "vitest";
import { allocationRows } from "#/components/terminal/allocation-side";
import { allocationEntryStats } from "#/components/terminal/decision-format";
import type { TerminalModel } from "#/components/terminal/model";

const modelWithDecisions = (
	decisions: TerminalModel["decisions"],
): TerminalModel =>
	({
		wallet: {
			available: "€1000.00",
			reserved: "€0.00",
		},
		decisions,
	}) as TerminalModel;

describe("allocationRows", () => {
	it("uses median + mad threshold like the tmp allocation x-ray", () => {
		const scores = [0.2, 0.4, 0.6, 0.8];
		const stats = allocationEntryStats(scores);
		const alloc = allocationRows(
			modelWithDecisions(
				scores.map((score, index) => ({
					key: `SYM${index}`,
					symbol: `SYM${index}/USD`,
					source: "pumpdump",
					scoreText: score.toFixed(3),
					scoreValue: score,
					verdict: "allow",
					why: "",
					edgeText: "",
					edgePositive: true,
					signals: [],
				})),
			),
		);

		expect(alloc.threshold).toBe(stats.threshold);
		expect(alloc.threshold).toBeCloseTo(0.8, 5);
	});

	it("allocates edge-proportional notional from deployable free cash", () => {
		const alloc = allocationRows(
			modelWithDecisions([
				{
					key: "SYM0",
					symbol: "SYM0/USD",
					source: "pumpdump",
					scoreText: "0.200",
					scoreValue: 0.2,
					verdict: "blocked",
					why: "",
					edgeText: "",
					edgePositive: false,
					signals: [],
				},
				{
					key: "SYM1",
					symbol: "SYM1/USD",
					source: "pumpdump",
					scoreText: "0.600",
					scoreValue: 0.6,
					verdict: "blocked",
					why: "",
					edgeText: "",
					edgePositive: false,
					signals: [],
				},
				{
					key: "SYM2",
					symbol: "SYM2/USD",
					source: "pumpdump",
					scoreText: "0.950",
					scoreValue: 0.95,
					verdict: "allow",
					why: "",
					edgeText: "",
					edgePositive: true,
					signals: [],
				},
			]),
		);

		expect(alloc.candidates.some((candidate) => candidate.allocated)).toBe(
			true,
		);
		expect(alloc.deployed).toBeGreaterThan(0);
		expect(alloc.deployedPercent).toBeGreaterThan(0);
	});
});
