// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { clockAtom, signals } from "#/collections/app";
import { ingestEnvelope, stores } from "#/collections/envelope";
import { Value } from "#/types/value";
import { useImpulseData } from "./use-impulse-data";

const SYMBOL = "BTC/USD";

const publishRegions = (
	symbol: string,
	rows: Array<Record<string, unknown>>,
) => {
	act(() => {
		ingestEnvelope({
			regions: {
				[symbol]: rows,
			},
		});
	});
};

const publishLabels = (
	origin: string,
	scope: string,
	labels: Record<string, number>,
) => {
	act(() => {
		ingestEnvelope({
			[origin]: {
				[scope]: labels,
			},
		});
	});
};

const publishTick = (tick: bigint) => {
	act(() => {
		signals.workspace.setState((prev) => ({
			...prev,
			aggregate: new Value({
				origin: "workspace",
				role: "tick",
				scope: "aggregate",
				prefix: "workspace/tick/aggregate",
				tick,
			}),
		}));
	});
};

const snm = (raw: number) => Math.abs(raw) / (1 + Math.abs(raw));

const pin = (data: ReturnType<typeof useImpulseData>["data"], id: string) =>
	data.find((node) => node.id === id);

const flushFrames = async () => {
	await act(async () => {
		await new Promise<void>((resolve) => {
			requestAnimationFrame(() => resolve());
		});
		await new Promise<void>((resolve) => {
			requestAnimationFrame(() => resolve());
		});
	});
};

afterEach(() => {
	cleanup();
	signals.workspace.setState(() => ({}));
	signals.training.setState(() => ({}));
	stores.regions?.setState(() => ({}));
	for (const origin of [
		"correlation",
		"pumpdump",
		"hawkes",
		"cvd",
		"liquidity",
		"sentiment",
	]) {
		stores[origin]?.setState(() => ({}));
	}
	clockAtom.set(null);
});

describe("useImpulseData", () => {
	it("paints a symbol's grid regions onto the map", () => {
		publishRegions(SYMBOL, [
			{
				symbol: SYMBOL,
				key: 1,
				brightness: 0.9,
				counts: 2,
			},
		]);

		const { result } = renderHook(() => useImpulseData(SYMBOL));
		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0.9);
		expect(result.current.litSymbols).toEqual([SYMBOL]);
		const lit = result.current.data.filter(
			(node) => node.cluster === 1 && (node.activation ?? 0) >= 0.9,
		);
		expect(lit.length).toBeGreaterThan(0);
	});

	it("zeros a region when the next tick's lit set omits it", () => {
		publishRegions(SYMBOL, [
			{
				symbol: SYMBOL,
				key: 1,
				brightness: 0.9,
				counts: 2,
			},
		]);

		const { result } = renderHook(() => useImpulseData(SYMBOL));
		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0.9);

		publishRegions(SYMBOL, [
			{
				symbol: SYMBOL,
				key: 2,
				brightness: 0.4,
				counts: 1,
			},
		]);

		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0);
		expect(result.current.regions.find((r) => r.id === 2)?.strength).toBe(0.4);
	});

	it("paints each symbol's grid frame in succession without merging", async () => {
		publishRegions("CFG/USD", [
			{
				symbol: "CFG/USD",
				key: 8,
				brightness: 1.625,
				counts: 3,
			},
		]);
		publishRegions("KAVA/USD", [
			{
				symbol: "KAVA/USD",
				key: 12,
				brightness: 2.4,
				counts: 3,
			},
		]);

		const { result } = renderHook(() => useImpulseData());
		await flushFrames();

		// Last painted symbol wins — values are not max-merged across the board.
		expect(result.current.litSymbols).toEqual(["KAVA/USD"]);
		expect(result.current.regions.find((r) => r.id === 12)?.strength).toBe(2.4);
		expect(result.current.regions.find((r) => r.id === 8)?.strength).toBe(0);
	});

	it("goes dark when a symbol publishes an empty regions frame", () => {
		publishRegions(SYMBOL, [
			{ symbol: SYMBOL, key: 1, brightness: 0.9, counts: 2 },
		]);

		const { result } = renderHook(() => useImpulseData(SYMBOL));
		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0.9);

		publishRegions(SYMBOL, []);

		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0);
	});

	it("reads pin metrics for the painted symbol only", () => {
		publishRegions(SYMBOL, []);
		publishLabels("correlation", "aggregate", {
			structural_alignment: 0.8,
		});

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		expect(
			pin(result.current.data, "correlation:structural_alignment:R01")
				?.activation,
		).toBeCloseTo(snm(0.8));
		// Region bars come from the grid frame, not from inventing pin peaks.
		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0);
	});

	it("does not create pins for hawkes metrics declared unpinned on the grid", () => {
		publishRegions(SYMBOL, []);
		publishLabels("hawkes", SYMBOL, {
			conditional_intensity: 1.2,
		});
		publishLabels("correlation", "aggregate", {
			structural_alignment: 0.8,
		});

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		expect(pin(result.current.data, "hawkes:conditional_intensity:R00")).toBeUndefined();
		expect(
			pin(result.current.data, "correlation:structural_alignment:R01")
				?.activation,
		).toBeCloseTo(snm(0.8));
	});

	it("advances marketTick when the aggregate tick moves", () => {
		publishRegions(SYMBOL, []);
		publishTick(1n);

		const { result } = renderHook(() => useImpulseData(SYMBOL));
		expect(result.current.marketTick).toBe(1);

		publishTick(2n);

		expect(result.current.marketTick).toBe(2);
	});
});
