import { afterEach, describe, expect, it } from "vitest";
import { signals } from "#/collections/app";
import type { Position } from "#/collections/types";

const pos = (id: string): Position =>
	({
		id,
		symbol: id,
		tier_key: "paper",
		entry_path: "",
		exit_path: "",
		opened_at: "2026-01-01T00:00:00Z",
	}) as Position;

afterEach(() => {
	signals.positions.setState(() => ({}));
});

describe("signals.positions", () => {
	it("stores a merge", () => {
		signals.positions.setState(() => ({ a: pos("a") }));
		signals.positions.setState((prev) => ({
			...(prev ?? {}),
			b: pos("b"),
		}));
		expect(Object.keys(signals.positions.state).sort()).toEqual(["a", "b"]);
	});
});
