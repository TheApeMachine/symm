import { describe, expect, it } from "vitest";
import type { TradeObservation } from "#/types/thesis";
import {
	auditObservations,
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
