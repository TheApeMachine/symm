import { afterEach, describe, expect, it } from "vitest";
import { signals } from "#/collections/app";
import { openLots, positionRows } from "#/components/learning/open-positions";
import { encodeJsonBytes } from "#/lib/json-bytes";
import { ingest } from "#/providers/websocket";
import { Value } from "#/types/value";

const book = (rows: Record<string, unknown>[], tick: bigint, scope = "all") => {
	const prefix = `positions/update/${scope}`;
	const value = new Value({
		origin: "positions",
		role: "update",
		scope,
		prefix,
		tick,
	});
	value.layers = [encodeJsonBytes(rows)];
	value.index = [`${prefix}/positions`];
	return value;
};

const lot = (
	id: string,
	symbol: string,
	mark: string,
	over: Record<string, unknown> = {},
) => ({
	id,
	symbol,
	virtual: false,
	opened_at: "2026-10-18T08:00:00Z",
	closed_at: null,
	qty: "1",
	mark,
	...over,
});

afterEach(() => {
	signals.positions.setState(() => ({}));
});

describe("position book", () => {
	it("keeps lots a sparse tick does not mention, in symbol order", () => {
		ingest([
			book(
				[
					lot("edge", "EDGE/USD", "1"),
					lot("aster", "ASTER/USD", "2"),
					lot("ptb", "PTB/USD", "3"),
				],
				1n,
			),
		]);
		ingest([book([lot("edge", "EDGE/USD", "1.4")], 2n)]);

		const rows = positionRows(signals.positions.state);
		expect(rows.map((row) => row.symbol)).toEqual([
			"ASTER/USD",
			"EDGE/USD",
			"PTB/USD",
		]);
		expect(rows.find((row) => row.id === "edge")?.mark).toBe("1.4");
		expect(rows.find((row) => row.id === "aster")?.mark).toBe("2");
		expect(openLots(rows, "paper")).toEqual([
			"ASTER/USD",
			"EDGE/USD",
			"PTB/USD",
		]);
	});

	it("lets a newer close drop one lot without clearing the book", () => {
		ingest([
			book([lot("aster", "ASTER/USD", "2"), lot("edge", "EDGE/USD", "1")], 1n),
		]);
		ingest([
			book(
				[
					lot("edge", "EDGE/USD", "1", {
						closed_at: "2026-10-18T09:00:00Z",
					}),
				],
				2n,
			),
		]);

		const rows = positionRows(signals.positions.state);
		expect(openLots(rows, "paper")).toEqual(["ASTER/USD"]);
	});

	it("replaces the book from a connect snapshot", () => {
		ingest([
			book([lot("aster", "ASTER/USD", "2"), lot("edge", "EDGE/USD", "1")], 1n),
		]);
		ingest([book([lot("aster", "ASTER/USD", "2.5")], 2n, "all")]);
		const snapshot = book([lot("ptb", "PTB/USD", "4")], 3n);
		snapshot.role = "snapshot";
		ingest([snapshot]);

		expect(
			positionRows(signals.positions.state).map((row) => row.symbol),
		).toEqual(["PTB/USD"]);
	});

	it("ignores an older tick that would shrink the book", () => {
		ingest([
			book([lot("aster", "ASTER/USD", "2"), lot("edge", "EDGE/USD", "9")], 5n),
		]);
		ingest([book([lot("edge", "EDGE/USD", "1")], 4n)]);

		const rows = positionRows(signals.positions.state);
		expect(rows.map((row) => row.symbol)).toEqual(["ASTER/USD", "EDGE/USD"]);
		expect(rows.find((row) => row.id === "edge")?.mark).toBe("9");
	});
});
