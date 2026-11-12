import { describe, expect, it } from "vitest";
import type { Position } from "#/collections/types";
import { mergePositionBook } from "./position-book";

const lot = (partial: Partial<Position> & Pick<Position, "id" | "symbol">): Position => ({
	entry_path: "exchange",
	exit_path: "",
	opened_at: "2026-11-11T20:12:38Z",
	tier_key: "real",
	...partial,
});

describe("mergePositionBook", () => {
	it("drops an open lot once the same trip is closed under a new id", () => {
		const open = lot({ id: "mon-open", symbol: "MON/USD", pnl_pct: "-3.2" });
		const closed = lot({
			id: "mon-closed",
			symbol: "MON/USD",
			closed_at: "2026-11-11T23:29:02Z",
			pnl_pct: "0.67",
		});
		const other = lot({
			id: "bera",
			symbol: "BERA/USD",
			opened_at: "2026-11-11T18:00:00Z",
		});

		const next = mergePositionBook(
			{ "mon-open": open, bera: other },
			{ "mon-closed": closed },
			{},
		);

		expect(next["mon-open"]).toBeUndefined();
		expect(next["mon-closed"]?.closed_at).toBe("2026-11-11T23:29:02Z");
		expect(next.bera?.symbol).toBe("BERA/USD");
	});

	it("replaces one tier on a snapshot and keeps the other books", () => {
		const paper = lot({
			id: "paper-doge",
			symbol: "DOGE/USD",
			tier_key: "paper",
		});
		const stale = lot({ id: "mon-open", symbol: "MON/USD" });
		const live = lot({
			id: "bera",
			symbol: "BERA/USD",
			opened_at: "2026-11-11T18:00:00Z",
		});

		const next = mergePositionBook(
			{ "paper-doge": paper, "mon-open": stale },
			{ bera: live },
			{ snapshot: true, tier: "real" },
		);

		expect(next["mon-open"]).toBeUndefined();
		expect(next.bera?.symbol).toBe("BERA/USD");
		expect(next["paper-doge"]?.symbol).toBe("DOGE/USD");
	});

	it("keeps an open lot when the close is a later trip", () => {
		const open = lot({
			id: "mon-next",
			symbol: "MON/USD",
			opened_at: "2026-11-11T23:40:00Z",
		});
		const closed = lot({
			id: "mon-closed",
			symbol: "MON/USD",
			opened_at: "2026-11-11T20:12:38Z",
			closed_at: "2026-11-11T23:29:02Z",
		});

		const next = mergePositionBook({}, { "mon-next": open, "mon-closed": closed }, {});

		expect(next["mon-next"]?.symbol).toBe("MON/USD");
		expect(next["mon-closed"]?.closed_at).toBe("2026-11-11T23:29:02Z");
	});
});
