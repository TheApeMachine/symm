import { bench, describe } from "vitest";

import type { FieldSnapshotEvent } from "#/lib/symm/events";
import { pickMarketWatchSymbol } from "#/lib/symm/events";

const field: FieldSnapshotEvent = {
	event: "field_snapshot",
	ts: "2026-05-23T12:00:00.000000000Z",
	symbol_count: 256,
	field: { div: 0, vort: 0, turb: 0, visc: 0, re: 0 },
	symbols: Array.from({ length: 256 }, (_, index) => ({
		symbol: `SYM${index}/EUR`,
		change_pct: 0,
		vol: 0,
		div: 0,
		vort: 0,
		turb: 0,
		visc: 0,
		re: index,
	})),
};

describe("pickMarketWatchSymbol", () => {
	bench("sticky scan over 256 field rows", () => {
		pickMarketWatchSymbol(undefined, field, "BTC/EUR", "SYM128/EUR");
	});
});
