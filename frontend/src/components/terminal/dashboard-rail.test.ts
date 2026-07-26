import { describe, expect, it } from "vitest";
import type { Holding } from "#/collections/types";
import { holdingRows } from "#/components/terminal/holding-wire";
import type { StrategyDecision } from "#/types/thesis";
import {
	auditHoldings,
	decisionFraction,
	holdingAuditRow,
	isClosedLot,
	paintDashboardHoldings,
} from "./dashboard-rail";

const samplePosition = (overrides: Partial<Holding> = {}): Holding => ({
	symbol: "BTC/USD",
	qty: 0,
	entry_price: 100,
	entry_fee: 0.1,
	exit_fee: 0.1,
	mark: 101,
	pnl: 0.5,
	return_pct: 0.01,
	status: "closed",
	...overrides,
});

const sampleDecision = (
	overrides: Partial<StrategyDecision> = {},
): StrategyDecision => ({
	action: "enter",
	symbol: "BTC/USD",
	at: "2026-07-14T12:00:00Z",
	utility: 0.42,
	alternatives: {},
	allocationClass: "core",
	proposedNotional: "100",
	proposedQuantity: "0.01",
	referencePrice: "61000",
	validThroughEpoch: 3,
	forecastSource: "resonance",
	forecastModel: "online",
	forecastEpoch: 2,
	calibrationCount: 4,
	expectedReturn: "0.01",
	expectedFees: "0.0002",
	expectedSpread: "0.0001",
	expectedImpact: "0.0003",
	adverseSelection: 0.0001,
	uncertainty: 0.05,
	confidence: 0.8,
	opportunityMargin: 0.01,
	cognitiveLead: 0.2,
	basinConfidence: 0.6,
	availableCapital: "1000",
	openPositions: 1,
	slotCapacity: 4,
	cause: "edge_clear",
	reason: "utility exceeds hold",
	...overrides,
});

describe("decisionFraction", () => {
	it("returns the sized notional share of available capital", () => {
		expect(decisionFraction(sampleDecision())).toBeCloseTo(0.1);
	});

	it("returns null for non-finite capital instead of throwing", () => {
		expect(
			decisionFraction(sampleDecision({ availableCapital: "NaN" })),
		).toBeNull();
	});

	it("accepts decimal wire strings without throwing DomainError", () => {
		expect(
			decisionFraction(
				sampleDecision({
					availableCapital: "1000.00",
					proposedNotional: "100",
				}),
			),
		).toBeCloseTo(0.1);
	});
});

describe("isClosedLot", () => {
	it("accepts closed holdings only", () => {
		expect(isClosedLot(samplePosition({ status: "closed" }))).toBe(true);
		expect(isClosedLot(samplePosition({ status: "open", qty: 1 }))).toBe(false);
	});
});

describe("auditHoldings", () => {
	it("keeps closed lots only", () => {
		expect(
			auditHoldings([
				samplePosition({ symbol: "BTC/USD", status: "closed" }),
				samplePosition({ symbol: "ETH/USD", status: "open", qty: 1 }),
			]),
		).toEqual([samplePosition({ symbol: "BTC/USD", status: "closed" })]);
	});
});

describe("holdingAuditRow", () => {
	it("formats a closed lot as an audit row", () => {
		const row = holdingAuditRow(
			samplePosition({ symbol: "BTC/USD", status: "closed", pnl: 0.5 }),
		);

		expect(row.reference).toBe("BTC/USD");
		expect(row.meta).toContain("pnl");
	});
});

describe("paintDashboardHoldings", () => {
	it("accepts backend map-shaped open holdings with decimal strings", () => {
		const rows = holdingRows({
			"EUL/USD": samplePosition({
				symbol: "EUL/USD",
				qty: "9.85795428",
				status: "open",
				pnl: "0",
			}),
			"ESPORTS/USD": samplePosition({
				symbol: "ESPORTS/USD",
				qty: "544.58996",
				status: "open",
				pnl: "0",
			}),
		});

		expect(rows.map((row) => row.symbol)).toEqual(["EUL/USD", "ESPORTS/USD"]);
		expect(rows[0]?.qty).toBe(9.85795428);
		expect(rows[1]?.qty).toBe(544.58996);
		expect(() => paintDashboardHoldings(rows, "BTC/USD")).not.toThrow();
	});
});
