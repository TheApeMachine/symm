import { afterEach, describe, expect, it } from "vitest";
import {
	applyPositionsSnapshot,
	direct,
	signals,
} from "#/collections/app";
import type { Position } from "#/collections/types";

const pos = (
	id: string,
	tier_key: "paper" | "virtual" | "real" = "paper",
): Position =>
	({
		id,
		symbol: id,
		tier_key,
		entry_path: "",
		exit_path: "",
		opened_at: "2026-01-01T00:00:00Z",
	}) as Position;

afterEach(() => {
	signals.positions.setState(() => ({}));
});

describe("applyPositionsSnapshot", () => {
	it("adds incoming ids for the tier", () => {
		const next = applyPositionsSnapshot({}, { a: pos("a") }, "paper");
		expect(Object.keys(next)).toEqual(["a"]);
	});

	it("drops tier ids omitted from the snapshot", () => {
		const prev = { a: pos("a"), b: pos("b") };
		const next = applyPositionsSnapshot(prev, { b: pos("b") }, "paper");
		expect(Object.keys(next).sort()).toEqual(["b"]);
	});

	it("clears the tier when Hello ships an empty open book", () => {
		const prev = { a: pos("a"), v: pos("v", "virtual") };
		const next = applyPositionsSnapshot(prev, {}, "paper");
		expect(Object.keys(next)).toEqual(["v"]);
	});

	it("preserves other tiers while replacing one", () => {
		const prev = {
			p: pos("p", "paper"),
			r: pos("r", "real"),
		};
		const next = applyPositionsSnapshot(
			prev,
			{ p2: pos("p2", "paper") },
			"paper",
		);
		expect(Object.keys(next).sort()).toEqual(["p2", "r"]);
	});
});

describe("direct.positions", () => {
	it("snapshot replace removes an exited id from the store", () => {
		direct.positions(
			{ keep: pos("keep"), gone: pos("gone") },
			{ positions_snapshot: true, tier: "paper" },
		);
		expect(Object.keys(signals.positions.state).sort()).toEqual([
			"gone",
			"keep",
		]);

		// Exit Hello omits BUSY/closed ids — store must drop them.
		direct.positions(
			{ keep: pos("keep") },
			{ positions_snapshot: true, tier: "paper" },
		);
		expect(Object.keys(signals.positions.state)).toEqual(["keep"]);
	});

	it("non-snapshot merges without deleting peers", () => {
		direct.positions(
			{ a: pos("a") },
			{ positions_snapshot: true, tier: "paper" },
		);
		direct.positions({ b: pos("b") }, { tier: "paper" });
		expect(Object.keys(signals.positions.state).sort()).toEqual(["a", "b"]);
	});
});
