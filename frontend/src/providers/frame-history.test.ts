import { describe, expect, it } from "vitest";
import { FrameHistory } from "#/providers/frame-history";

const measurement = (
	at: string,
	raw: number,
	symbol = "BTC/USD",
	source = "hawkes",
	metric = "intensity",
) => ({ at, metric, raw, source, symbol });

describe("FrameHistory", () => {
	it("appends observations across websocket frames", () => {
		const history = new FrameHistory(() => 3);

		history.retain("measurements", [measurement("2026-07-21T10:00:00Z", 1)]);
		const retained = history.retain("measurements", [
			measurement("2026-07-21T10:00:01Z", 2),
		]) as Array<{ raw: number }>;

		expect(retained.map((row) => row.raw)).toEqual([1, 2]);
	});

	it("updates a replayed timestamp instead of duplicating it", () => {
		const history = new FrameHistory(() => 3);

		history.retain("measurements", [measurement("2026-07-21T10:00:00Z", 1)]);
		const retained = history.retain("measurements", [
			measurement("2026-07-21T10:00:00Z", 7),
		]) as Array<{ raw: number }>;

		expect(retained).toHaveLength(1);
		expect(retained[0]?.raw).toBe(7);
	});

	it("inserts delayed fit observations in timestamp order", () => {
		const history = new FrameHistory(() => 3);

		history.retain("measurements", [measurement("2026-07-21T10:00:02Z", 2)]);
		const retained = history.retain("measurements", [
			measurement("2026-07-21T10:00:00Z", 0),
			measurement("2026-07-21T10:00:01Z", 1),
		]) as Array<{ raw: number }>;

		expect(retained.map((row) => row.raw)).toEqual([0, 1, 2]);
	});

	it("bounds each entity independently and keeps the newest timestamps", () => {
		const history = new FrameHistory(() => 2);

		const retained = history.retain("measurements", [
			measurement("2026-07-21T10:00:00Z", 0),
			measurement("2026-07-21T10:00:01Z", 1),
			measurement("2026-07-21T10:00:02Z", 2),
			measurement("2026-07-21T10:00:00Z", 8, "ETH/USD"),
		]) as Array<{ raw: number }>;

		expect(retained.map((row) => row.raw)).toEqual([8, 1, 2]);
	});

	it("retains independent metrics reported at the same timestamp", () => {
		const history = new FrameHistory(() => 2);

		const retained = history.retain("measurements", [
			measurement("2026-07-21T10:00:00Z", 1, "BTC/USD", "hawkes", "mu"),
			measurement("2026-07-21T10:00:00Z", 2, "BTC/USD", "hawkes", "lambda"),
		]) as Array<{ raw: number }>;

		expect(retained.map((row) => row.raw)).toEqual([1, 2]);
	});

	it("passes snapshot streams through without copying", () => {
		const history = new FrameHistory(() => 2);
		const balances = [{ asset: "USD", available: 10 }];

		expect(history.retain("balances", balances)).toBe(balances);
	});

	it("rejects malformed temporal rows visibly", () => {
		const history = new FrameHistory(() => 2);

		expect(() =>
			history.retain("measurements", [{ symbol: "BTC/USD" }]),
		).toThrow("measurements history row requires source");
	});
});
