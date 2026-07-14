import { describe, expect, it } from "vitest";
import { executionAuditRow } from "./dashboard-rail";

describe("executionAuditRow", () => {
	it("formats a flat execution as an immediate audit row", () => {
		expect(
			executionAuditRow({
				exec_id: "E1",
				exec_type: "trade",
				order_status: "filled",
				symbol: "BTC/USD",
				side: "buy",
				last_qty: 0.01,
				last_price: 61420,
				timestamp: "2026-07-12T04:05:06Z",
				sequence: 42,
			}),
		).toEqual({
			reason: "filled",
			reference: "#42 · 04:05:06",
			meta: "trade · buy · BTC/USD · 0.01 @ 61420.000",
		});
	});
});
