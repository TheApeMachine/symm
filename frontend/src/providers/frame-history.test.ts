import { describe, expect, it } from "vitest";
import { FrameHistory, frameRows } from "#/providers/frame-history";

const measurement = (
	at: string,
	raw: number,
	symbol = "BTC/USD",
	source = "hawkes",
	metric = "intensity",
) => ({ at, metric, raw, source, symbol });

const focusedHistory = (capacity: number) =>
	new FrameHistory(
		() => capacity,
		() => "BTC/USD",
	);

describe("FrameHistory", () => {
	it("appends observations across websocket frames", () => {
		const history = focusedHistory(3);

		history.retain("measurements", [measurement("2026-07-21T10:00:00Z", 1)]);
		history.retain("measurements", [measurement("2026-07-21T10:00:01Z", 2)]);
		const retained = history.values("measurements");

		expect(retained.map((row) => row.raw)).toEqual([1, 2]);
	});

	it("updates a replayed timestamp instead of duplicating it", () => {
		const history = focusedHistory(3);

		history.retain("measurements", [measurement("2026-07-21T10:00:00Z", 1)]);
		history.retain("measurements", [measurement("2026-07-21T10:00:00Z", 7)]);
		const retained = history.values("measurements");

		expect(retained).toHaveLength(1);
		expect(retained[0]?.raw).toBe(7);
	});

	it("inserts delayed fit observations in timestamp order", () => {
		const history = focusedHistory(3);

		history.retain("measurements", [measurement("2026-07-21T10:00:02Z", 2)]);
		history.retain("measurements", [
			measurement("2026-07-21T10:00:00Z", 0),
			measurement("2026-07-21T10:00:01Z", 1),
		]);
		const retained = history.values("measurements");

		expect(retained.map((row) => row.raw)).toEqual([0, 1, 2]);
	});

	it("bounds focused history and keeps the newest cross-section rows", () => {
		const history = focusedHistory(2);

		history.retain("measurements", [
			measurement("2026-07-21T10:00:00Z", 0),
			measurement("2026-07-21T10:00:01Z", 1),
			measurement("2026-07-21T10:00:02Z", 2),
			measurement("2026-07-21T10:00:00Z", 8, "ETH/USD"),
		]);
		const retained = history.values("measurements");

		expect(retained.map((row) => row.raw)).toEqual([1, 2, 8]);
	});

	it("retains independent metrics reported at the same timestamp", () => {
		const history = focusedHistory(2);

		history.retain("measurements", [
			measurement("2026-07-21T10:00:00Z", 1, "BTC/USD", "hawkes", "mu"),
			measurement("2026-07-21T10:00:00Z", 2, "BTC/USD", "hawkes", "lambda"),
		]);
		const retained = history.values("measurements");

		expect(retained.map((row) => row.raw)).toEqual([1, 2]);
	});

	it("does not retain snapshot streams", () => {
		const history = focusedHistory(2);
		const balances = [{ asset: "USD", available: 10 }];

		history.retain("balances", balances);

		expect(history.values("balances")).toEqual([]);
	});

	it("keeps only the latest history row outside the focused symbol", () => {
		const history = focusedHistory(3);

		history.retain("measurements", [
			measurement("2026-07-21T10:00:00Z", 1, "ETH/USD"),
			measurement("2026-07-21T10:00:01Z", 2, "ETH/USD"),
		]);

		expect(history.values("measurements").map((row) => row.raw)).toEqual([2]);
	});

	it("moves the temporal budget when focus changes", () => {
		let focusSymbol = "BTC/USD";
		const history = new FrameHistory(
			() => 3,
			() => focusSymbol,
		);

		history.retain("measurements", [
			measurement("2026-07-21T10:00:00Z", 1),
			measurement("2026-07-21T10:00:01Z", 2),
			measurement("2026-07-21T10:00:00Z", 8, "ETH/USD"),
		]);
		focusSymbol = "ETH/USD";
		history.retain("measurements", [
			measurement("2026-07-21T10:00:02Z", 3),
			measurement("2026-07-21T10:00:01Z", 9, "ETH/USD"),
		]);

		expect(history.values("measurements").map((row) => row.raw)).toEqual([
			3, 8, 9,
		]);
	});

	it("keeps model-state streams at one latest row per symbol", () => {
		const history = focusedHistory(3);

		history.retain("causal", [
			{ at: "2026-07-21T10:00:00Z", symbol: "BTC/USD", strength: 1 },
			{ at: "2026-07-21T10:00:01Z", symbol: "BTC/USD", strength: 2 },
		]);

		expect(history.values("causal").map((row) => row.strength)).toEqual([2]);
	});

	it("projects retained rows in the requested shape", () => {
		const history = focusedHistory(2);
		history.retain("measurements", [measurement("2026-07-21T10:00:00Z", 1)]);
		const projection = history.project("measurements", "history");

		expect(
			frameRows<{ raw: number }>(projection).map((row) => row.raw),
		).toEqual([1]);
	});

	it("retains the latest cognitive reading per symbol", () => {
		const history = focusedHistory(2);

		history.retain("cognition", [
			{ at: "2026-07-21T10:00:00Z", symbol: "BTC/USD", cohort: 1 },
			{ at: "2026-07-21T10:00:01Z", symbol: "BTC/USD", cohort: 2 },
		]);

		expect(history.latest("cognition").map((row) => row.cohort)).toEqual([2]);
	});

	it("projects only the newest observation for cross-sectional painters", () => {
		const history = focusedHistory(3);

		history.retain("measurements", [
			measurement("2026-07-21T10:00:00Z", 1),
			measurement("2026-07-21T10:00:01Z", 2),
			measurement("2026-07-21T10:00:00Z", 8, "ETH/USD"),
		]);

		expect(history.latest("measurements").map((row) => row.raw)).toEqual([
			2, 8,
		]);
	});

	it("rejects malformed temporal rows visibly", () => {
		const history = focusedHistory(2);

		expect(() =>
			history.retain("measurements", [{ symbol: "BTC/USD" }]),
		).toThrow("measurements history row requires source");
	});
});
