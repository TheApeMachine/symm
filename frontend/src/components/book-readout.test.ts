import { describe, expect, it } from "vitest";
import type { Position } from "#/collections/types";
import { holdNs, isClosed, onTier, pnlFrac, pnlPct } from "./book-readout";

const lot = (partial: Partial<Position>): Position => ({
	id: "a",
	symbol: "DOGE/USD",
	entry_path: "R01",
	exit_path: "",
	opened_at: "2026-01-01T00:00:00Z",
	...partial,
});

describe("Position wire fields", () => {
	it("reads qty, entry, mark, pnl_pct from the lot", () => {
		const pos = lot({
			qty: "10",
			entry: "0.20",
			mark: "0.22",
			pnl_pct: "8",
		});

		expect(pnlPct(pos)).toBe(8);
		expect(pnlFrac(pos)).toBeCloseTo(0.08, 8);
		expect(isClosed(pos)).toBe(false);
	});

	it("treats closed_at as closed", () => {
		const pos = lot({
			closed_at: "2026-01-01T00:01:00Z",
			pnl_pct: "8",
		});

		expect(isClosed(pos)).toBe(true);
		expect(holdNs(pos)).toBe(60_000_000_000);
	});
});

describe("onTier", () => {
	it("defaults unlabeled lots to paper", () => {
		expect(onTier(lot({}), "paper")).toBe(true);
		expect(onTier(lot({}), "virtual")).toBe(false);
		expect(onTier(lot({ tier_key: "virtual" }), "virtual")).toBe(true);
	});
});
