import { describe, expect, it } from "vitest";
import {
	allocationModelFromStores,
	allocationRows,
} from "#/components/terminal/allocation-side";

const modelWithDecisions = (
	decisions: Record<string, unknown>[],
) =>
	({
		wallet: {
			available: "€1000.00",
			reserved: "€0.00",
		},
		decisions,
	});

describe("allocationRows", () => {
	it("renders backend decision score and allocator fraction without deriving notional", () => {
		const alloc = allocationRows(
			modelWithDecisions([
				{
					uuid: "TON/USD:766",
					symbol: "TON/USD",
					score: 0.298,
					verdict: "allow",
					why: "admitted",
					fraction: 0.064,
					tick: 766,
				},
			]),
		);

		expect(alloc.candidates).toHaveLength(1);
		expect(alloc.candidates[0]?.symbol).toBe("TON/USD");
		expect(alloc.candidates[0]?.scoreValue).toBe(0.298);
		expect(alloc.candidates[0]?.fraction).toBe(0.064);
		expect(alloc.admittedCount).toBe(1);
		expect(alloc.deployed).toBe(0);
	});

	it("TestAllocationDoesNotCollapseDuplicateSymbols", () => {
		const alloc = allocationRows(
			modelWithDecisions([
				{
					uuid: "TON/USD:old",
					symbol: "TON/USD",
					score: 0.1,
					verdict: "blocked",
					why: "below edge",
					fraction: 0,
					tick: 765,
				},
				{
					uuid: "TON/USD:new",
					symbol: "TON/USD",
					score: 0.298,
					verdict: "allow",
					why: "admitted",
					fraction: 0.064,
					tick: 766,
				},
			]),
		);

		expect(alloc.candidates.map((candidate) => candidate.symbol)).toEqual([
			"TON/USD",
			"TON/USD",
		]);
		expect(alloc.candidates.map((candidate) => candidate.key)).toEqual([
			"TON/USD:old",
			"TON/USD:new",
		]);
	});

	it("uses backend decision batches as the allocation source", () => {
		const alloc = allocationModelFromStores(
			{
				quote: "USD",
				data: [{ asset: "USD", balance: 1000 }],
				reserved: 0,
			},
			{
				uuid: "decision-1",
				tick: 2,
				symbol: "TRADER/USD",
				verdict: "blocked",
				why: "field_risk",
				score: 0.24,
				fraction: 0,
			},
		);

		expect(alloc.candidates.map((candidate) => candidate.symbol)).toEqual([
			"TRADER/USD",
		]);
		expect(alloc.freeCash).toBe(1000);
		expect(alloc.candidates[0]?.scoreValue).toBe(0.24);
		expect(alloc.candidates[0]?.verdict).toBe("blocked");
	});

	it("uses decision artifact arrays as the allocation source", () => {
		const alloc = allocationModelFromStores(
			{
				quote: "USD",
				data: [{ asset: "USD", balance: 1000 }],
				reserved: 0,
			},
			[
				{
					uuid: "decision-eth",
					tick: 10,
					symbol: "ETH/USD",
					verdict: "allow",
					why: "admitted",
					score: 0.62,
					fraction: 0.05,
				},
			],
		);

		expect(alloc.candidates.map((candidate) => candidate.symbol)).toEqual([
			"ETH/USD",
		]);
	});

	it("uses backend positions for deployed capital instead of admitted decisions", () => {
		const alloc = allocationModelFromStores(
			{
				quote: "USD",
				data: [{ asset: "USD", balance: 200 }],
				reserved: 0,
			},
			{
				uuid: "decision-1",
				tick: 2,
				symbol: "TRADER/USD",
				verdict: "allow",
				why: "admitted",
				score: 1,
				fraction: 0.05,
			},
			{
				role: "positions",
				positions: [],
			},
		);

		expect(alloc.admittedCount).toBe(1);
		expect(alloc.deployed).toBe(0);
		expect(alloc.positionCount).toBe(0);
	});

	it("counts actual deployed capital from backend position value", () => {
		const alloc = allocationModelFromStores(
			{
				quote: "USD",
				data: [{ asset: "USD", balance: 190 }],
				reserved: 0,
			},
			null,
			{
				role: "positions",
				positions: [{ symbol: "ALGO/USD", value: 10.85 }],
			},
		);

		expect(alloc.deployed).toBe(10.85);
		expect(alloc.positionCount).toBe(1);
		expect(alloc.deployedPercent).toBeGreaterThan(0);
	});

	it("TestAllocationDoesNotInferQuoteFromFirstAsset", () => {
		const alloc = allocationModelFromStores({
			data: [{ asset: "BTC", balance: 2 }],
			reserved: 0,
		});

		expect(alloc.quote).toBe("quote unavailable");
		expect(alloc.freeCash).toBe(0);
	});

	it("shows the backend blocker when no candidates are admitted", () => {
		const alloc = allocationModelFromStores(
			{
				quote: "USD",
				data: [{ asset: "USD", balance: 200 }],
			},
			[],
			null,
			{
				role: "decision_funnel",
				tick: 9,
				first_blocker: "holding:missing_source",
			},
		);

		expect(alloc.candidates).toHaveLength(0);
		expect(alloc.emptyReason).toBe("holding:missing source");
	});
});
