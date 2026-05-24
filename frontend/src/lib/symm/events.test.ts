import { describe, expect, it } from "vitest";

import type { FieldSnapshotEvent, ScoreboardEvent } from "#/lib/symm/events";
import { pickMarketWatchSymbol } from "#/lib/symm/events";

const fieldSnapshot = (
	symbols: Array<{ symbol: string; re: number }>,
): FieldSnapshotEvent => ({
	event: "field_snapshot",
	ts: "2026-05-23T12:00:00.000000000Z",
	symbol_count: symbols.length,
	field: { div: 0, vort: 0, turb: 0, visc: 0, re: 0 },
	symbols: symbols.map((row) => ({
		symbol: row.symbol,
		change_pct: 0,
		vol: 0,
		div: 0,
		vort: 0,
		turb: 0,
		visc: 0,
		re: row.re,
	})),
});

describe("pickMarketWatchSymbol", () => {
	it("keeps sticky symbol while it remains in the field", () => {
		const field = fieldSnapshot([
			{ symbol: "ETH/EUR", re: 120 },
			{ symbol: "BTC/EUR", re: 100 },
		]);

		expect(pickMarketWatchSymbol(undefined, field, "BTC/EUR", "BTC/EUR")).toBe(
			"BTC/EUR",
		);
	});

	it("prefers fallback when it has live ticks", () => {
		const field = fieldSnapshot([{ symbol: "ETH/EUR", re: 500 }]);
		const scoreboard: ScoreboardEvent = {
			event: "scoreboard",
			ts: "2026-05-23T12:00:00.000000000Z",
			line: 1,
			median: 1,
			mad: 1,
			targets: [
				{
					symbol: "SOL/EUR",
					regime: "pump",
					reason: "ok",
					score: 2,
					effective_score: 2,
					trail_pct: 0,
				},
			],
		};
		const hasTick = (symbol: string) => symbol === "BTC/EUR";

		expect(
			pickMarketWatchSymbol(scoreboard, field, "BTC/EUR", "ETH/EUR", hasTick),
		).toBe("BTC/EUR");
	});

	it("follows scoreboard target when fallback has no ticks", () => {
		const field = fieldSnapshot([{ symbol: "ETH/EUR", re: 500 }]);
		const scoreboard: ScoreboardEvent = {
			event: "scoreboard",
			ts: "2026-05-23T12:00:00.000000000Z",
			line: 1,
			median: 1,
			mad: 1,
			targets: [
				{
					symbol: "SOL/EUR",
					regime: "pump",
					reason: "ok",
					score: 2,
					effective_score: 2,
					trail_pct: 0,
				},
			],
		};
		const hasTick = (symbol: string) => symbol === "SOL/EUR";

		expect(
			pickMarketWatchSymbol(scoreboard, field, "BTC/EUR", "ETH/EUR", hasTick),
		).toBe("SOL/EUR");
	});

	it("returns the first field row with ticks when scoreboard is empty", () => {
		const field = fieldSnapshot([
			{ symbol: "ETH/EUR", re: 42 },
			{ symbol: "BTC/EUR", re: 42 },
		]);
		const hasTick = (symbol: string) =>
			symbol === "ETH/EUR" || symbol === "BTC/EUR";

		expect(
			pickMarketWatchSymbol(
				undefined,
				field,
				"MISSING/EUR",
				undefined,
				hasTick,
			),
		).toBe("ETH/EUR");
	});
});
