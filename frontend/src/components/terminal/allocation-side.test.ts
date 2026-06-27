import { describe, expect, it } from "vitest";
import {
	allocationModelFromStores,
	allocationRows,
} from "#/components/terminal/allocation-side";
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
					scoreText: "0.400",
					scoreValue: 0.4,
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
					scoreText: "0.900",
					scoreValue: 0.9,
					verdict: "in-play",
					why: "",
					edgeText: "",
					edgePositive: false,
					signals: [],
				},
				{
					key: "SYM3",
					symbol: "SYM3/USD",
					source: "pumpdump",
					scoreText: "1.000",
					scoreValue: 1,
					verdict: "allow",
					why: "",
					edgeText: "",
					edgePositive: true,
					signals: [],
				},
				{
					key: "SYM4",
					symbol: "SYM4/USD",
					source: "pumpdump",
					scoreText: "0.500",
					scoreValue: 0.5,
					verdict: "blocked",
					why: "",
					edgeText: "",
					edgePositive: false,
					signals: [],
				},
			]),
		);

		const allocated = alloc.candidates.find(
			(candidate) => candidate.symbol === "SYM3/USD",
		);
		const inPlay = alloc.candidates.find(
			(candidate) => candidate.symbol === "SYM2/USD",
		);

		expect(allocated?.allocated).toBe(true);
		expect(inPlay?.allocated).toBe(false);
		expect(inPlay?.share).toBeGreaterThan(0);
		expect(alloc.deployed).toBeGreaterThan(0);
		expect(alloc.deployed).toBeCloseTo(allocated?.notional ?? 0, 5);
		expect(alloc.deployedPercent).toBeGreaterThan(0);
		const positiveThesis = alloc.candidates.reduce(
			(sum, candidate) => sum + Math.max(0, candidate.scoreValue),
			0,
		);

		expect(allocated?.share).toBeCloseTo(
			allocated ? allocated.edge / (positiveThesis + allocated.scoreValue) : 0,
			5,
		);
	});

	it("uses backend decision batches instead of walk-derived scores when present", () => {
		const alloc = allocationModelFromStores(
			{
				asset: [{ asset: "USD", balance: 1000 }],
				reserved: 0,
			},
			{
				"WALK/USD": {
					symbol: "WALK/USD",
					steps: [{ path: [0], outcome: "action" }],
				},
			},
			{
				pumpdump: {
					"WALK/USD": {
						confidence: 1,
						output: { category: "vertical" },
					},
				},
			},
			{
				role: "decisions",
				seq: 2,
				decisions: [
					{
						action_id: "decision-1",
						symbol: "TRADER/USD",
						verdict: "blocked",
						why: "field_risk",
						confidence: 1,
						score: 0.24,
					},
				],
			},
		);

		expect(alloc.candidates.map((candidate) => candidate.symbol)).toEqual([
			"TRADER/USD",
		]);
		expect(alloc.candidates[0]?.scoreValue).toBe(0.24);
		expect(alloc.candidates[0]?.allocated).toBe(false);
	});
});
