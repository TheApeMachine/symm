import { afterEach, describe, expect, it } from "vitest";
import { signals } from "#/collections/app";
import type { Position } from "#/collections/types";
import { applyPositionsSnapshot } from "#/providers/websocket";

const pos = (id: string, tier = "paper"): Position =>
	({
		id,
		symbol: id,
		tier_key: tier,
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

	it("prunes exited positions on tier snapshot while preserving other tiers", () => {
		signals.positions.setState(() => ({
			p1: pos("p1", "paper"),
			p2: pos("p2", "paper"),
			r1: pos("r1", "real"),
		}));

		// paper sends snapshot with only p2 (p1 has exited)
		applyPositionsSnapshot({ p2: pos("p2", "paper") }, "paper");

		expect(Object.keys(signals.positions.state).sort()).toEqual(["p2", "r1"]);

		// paper closes all positions
		applyPositionsSnapshot({}, "paper");
		expect(Object.keys(signals.positions.state)).toEqual(["r1"]);
	});
});
