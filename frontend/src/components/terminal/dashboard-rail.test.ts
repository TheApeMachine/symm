import { describe, expect, it } from "vitest";
import type { StrategyDecision, TradeObservation } from "#/types/thesis";
import {
	auditObservations,
	decisionFraction,
	isAuditObservation,
	tradeObservationAuditRow,
} from "./dashboard-rail";

const observation = (
	overrides: Partial<TradeObservation>,
): TradeObservation => ({
	kind: "execution",
	symbol: "BTC/USD",
	decision: 0,
	at: "2026-07-12T04:05:06Z",
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
	proposedNotional: 100,
	proposedQuantity: 0.01,
	referencePrice: 61000,
	validThroughEpoch: 3,
	forecastSource: "resonance",
	forecastModel: "online",
	forecastEpoch: 2,
	calibrationCount: 4,
	expectedReturn: 0.01,
	expectedFees: 0.0002,
	expectedSpread: 0.0001,
	expectedImpact: 0.0003,
	adverseSelection: 0.0001,
	uncertainty: 0.05,
	confidence: 0.8,
	opportunityMargin: 0.01,
	cognitiveLead: 0.2,
	basinConfidence: 0.6,
	availableCapital: 1000,
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
			decisionFraction(sampleDecision({ availableCapital: Number.NaN })),
		).toBeNull();
	});

	it("accepts decimal wire strings without throwing DomainError", () => {
		expect(
			decisionFraction(
				sampleDecision({
					availableCapital: "1000.00" as unknown as number,
					proposedNotional: "100" as unknown as number,
				}),
			),
		).toBeCloseTo(0.1);
	});
});

describe("isAuditObservation", () => {
	it("accepts entry and exit journal rows only", () => {
		expect(
			isAuditObservation(
				observation({ kind: "execution", side: "buy", status: "filled" }),
			),
		).toBe(true);
		expect(
			isAuditObservation(
				observation({ kind: "execution", side: "sell", status: "filled" }),
			),
		).toBe(true);
		expect(
			isAuditObservation(
				observation({
					kind: "lifecycle_transition",
					status: "entered",
				}),
			),
		).toBe(true);
		expect(
			isAuditObservation(
				observation({
					kind: "lifecycle_transition",
					status: "closed",
				}),
			),
		).toBe(true);
		expect(
			isAuditObservation(
				observation({ kind: "broker_acceptance", action: "enter" }),
			),
		).toBe(false);
		expect(
			isAuditObservation(
				observation({
					kind: "lifecycle_transition",
					status: "managing",
				}),
			),
		).toBe(false);
	});
});

describe("auditObservations", () => {
	it("drops non-entry and non-exit journal rows", () => {
		expect(
			auditObservations([
				observation({ kind: "intent_submission", action: "enter" }),
				observation({ kind: "execution", side: "buy", status: "filled" }),
				observation({
					kind: "lifecycle_transition",
					status: "managing",
				}),
				observation({ kind: "execution", side: "sell", status: "filled" }),
			]),
		).toEqual([
			observation({ kind: "execution", side: "buy", status: "filled" }),
			observation({ kind: "execution", side: "sell", status: "filled" }),
		]);
	});
});

describe("tradeObservationAuditRow", () => {
	it("formats an entry fill as an audit row", () => {
		expect(
			tradeObservationAuditRow(
				observation({
					kind: "execution",
					status: "filled",
					side: "buy",
					executionId: "E1",
					quantity: "0.01",
					price: "61420",
				}),
			),
		).toEqual({
			reason: "filled",
			reference: "#E1 · 04:05:06",
			meta: "execution · buy · BTC/USD · 0.01 @ 61420.000",
		});
	});

	it("formats an exit lifecycle transition as an audit row", () => {
		expect(
			tradeObservationAuditRow(
				observation({
					kind: "lifecycle_transition",
					status: "closed",
				}),
			),
		).toEqual({
			reason: "closed",
			reference: "#0 · 04:05:06",
			meta: "lifecycle_transition · BTC/USD",
		});
	});
});
