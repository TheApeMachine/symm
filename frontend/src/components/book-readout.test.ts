import { describe, expect, it } from "vitest";
import { onBook, positionReadout } from "./book-readout";

describe("positionReadout", () => {
	it("derives qty and entry price from buy fills", () => {
		const readout = positionReadout({
			id: "a",
			symbol: "DOGE/USD",
			opened_at: "2026-01-01T00:00:00Z",
			fills: [
				{
					side: "buy",
					last_qty: "10",
					avg_price: "0.20",
					cost: "2",
					fee_usd_equiv: "0.01",
				},
			],
			mark: "0.22",
			pnl_pct: "8",
		});

		expect(readout.qty).toBe(10);
		expect(readout.entryPrice).toBeCloseTo(0.2, 8);
		expect(readout.entryFee).toBeCloseTo(0.01, 8);
		expect(readout.mark).toBe(0.22);
		expect(readout.returnPctDisplay).toBe(8);
		expect(readout.closed).toBe(false);
	});

	it("falls back to entry_order.volume when fills are missing", () => {
		const readout = positionReadout({
			id: "b",
			symbol: "APE/USD",
			opened_at: "2026-01-01T00:00:00Z",
			entry_order: { volume: "3.5", type: "buy", ordertype: "market" },
			mark: "1",
		});

		expect(readout.qty).toBe(3.5);
		expect(readout.entryPrice).toBeNull();
	});

	it("marks a lot closed once sell fills cover the buy qty", () => {
		const readout = positionReadout({
			id: "c",
			symbol: "XRP/USD",
			opened_at: "2026-01-01T00:00:00Z",
			fills: [
				{ side: "buy", last_qty: "2", avg_price: "1", cost: "2" },
				{
					side: "sell",
					last_qty: "2",
					avg_price: "1.1",
					cost: "2.2",
					timestamp: "2026-01-01T00:01:00Z",
				},
			],
			pnl_pct: "8",
		});

		expect(readout.closed).toBe(true);
		expect(readout.exitVwap).toBeCloseTo(1.1, 8);
		expect(readout.closedAt).toBe("2026-01-01T00:01:00.000Z");
	});
});

describe("onBook", () => {
	it("defaults unlabeled lots to paper", () => {
		expect(onBook({ id: "x" }, "paper")).toBe(true);
		expect(onBook({ id: "x" }, "virtual")).toBe(false);
		expect(onBook({ id: "x", book: "virtual" }, "virtual")).toBe(true);
	});
});
