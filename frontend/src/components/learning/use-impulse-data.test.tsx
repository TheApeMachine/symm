// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { clockAtom, ensureSignalStore, signals } from "#/collections/app";
import { Value } from "#/types/value";
import { useImpulseData } from "./use-impulse-data";

const SYMBOL = "BTC/USD";

const metricFrame = (
	origin: string,
	scope: string,
	layers: Array<{ key: string; payload: unknown }>,
	tick = 1n,
): Value => {
	const prefix = `${origin}/metric/${scope}`;
	return new Value({
		origin,
		role: "metric",
		scope,
		prefix,
		tick,
		index: layers.map((layer) => `${prefix}/${layer.key}`),
		layers: layers.map((layer) =>
			new TextEncoder().encode(JSON.stringify(layer.payload)),
		),
	});
};

const publish = (origin: string, scope: string, value: Value) => {
	act(() => {
		ensureSignalStore(origin).setState((prev) => ({
			...prev,
			[scope]: value,
		}));
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
	signals.grid?.setState(() => ({}));
	signals.train?.setState(() => ({}));
	for (const origin of [
		"correlation",
		"pumpdump",
		"hawkes",
		"cvd",
		"liquidity",
		"sentiment",
	]) {
		signals[origin]?.setState(() => ({}));
	}
	clockAtom.set(null);
});

describe("useImpulseData", () => {
	it("paints a symbol's grid regions onto the map", () => {
		publish(
			"grid",
			SYMBOL,
			metricFrame("grid", SYMBOL, [
				{
					key: "regions",
					payload: [
						{
							symbol: SYMBOL,
							key: 1,
							brightness: 0.9,
							counts: 2,
						},
					],
				},
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));
		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0.9);
		expect(result.current.litSymbols).toEqual([SYMBOL]);
		const lit = result.current.data.filter(
			(node) => node.cluster === 1 && (node.activation ?? 0) >= 0.9,
		);
		expect(lit.length).toBeGreaterThan(0);
	});

	it("zeros a region when the next tick's lit set omits it", () => {
		publish(
			"grid",
			SYMBOL,
			metricFrame("grid", SYMBOL, [
				{
					key: "regions",
					payload: [
						{
							symbol: SYMBOL,
							key: 1,
							brightness: 0.9,
							counts: 2,
						},
					],
				},
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));
		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0.9);

		publish(
			"grid",
			SYMBOL,
			metricFrame(
				"grid",
				SYMBOL,
				[
					{
						key: "regions",
						payload: [
							{
								symbol: SYMBOL,
								key: 2,
								brightness: 0.4,
								counts: 1,
							},
						],
					},
				],
				2n,
			),
		);

		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0);
		expect(result.current.regions.find((r) => r.id === 2)?.strength).toBe(0.4);
	});

	it("paints each symbol's grid frame in succession without merging", async () => {
		publish(
			"grid",
			"CFG/USD",
			metricFrame("grid", "CFG/USD", [
				{
					key: "regions",
					payload: [
						{
							symbol: "CFG/USD",
							key: 8,
							brightness: 1.625,
							counts: 3,
						},
					],
				},
			]),
		);
		publish(
			"grid",
			"KAVA/USD",
			metricFrame(
				"grid",
				"KAVA/USD",
				[
					{
						key: "regions",
						payload: [
							{
								symbol: "KAVA/USD",
								key: 12,
								brightness: 2.4,
								counts: 3,
							},
						],
					},
				],
				2n,
			),
		);

		const { result } = renderHook(() => useImpulseData());
		await flushFrames();

		// Last painted symbol wins — values are not max-merged across the board.
		expect(result.current.litSymbols).toEqual(["KAVA/USD"]);
		expect(result.current.regions.find((r) => r.id === 12)?.strength).toBe(2.4);
		expect(result.current.regions.find((r) => r.id === 8)?.strength).toBe(0);
	});

	it("goes dark when a symbol publishes an empty regions frame", () => {
		publish(
			"grid",
			SYMBOL,
			metricFrame("grid", SYMBOL, [
				{
					key: "regions",
					payload: [
						{ symbol: SYMBOL, key: 1, brightness: 0.9, counts: 2 },
					],
				},
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));
		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0.9);

		publish(
			"grid",
			SYMBOL,
			metricFrame(
				"grid",
				SYMBOL,
				[{ key: "regions", payload: [] }],
				2n,
			),
		);

		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0);
	});

	it("reads pin metrics for the painted symbol only", () => {
		publish(
			"grid",
			SYMBOL,
			metricFrame("grid", SYMBOL, [
				{ key: "regions", payload: [] },
			]),
		);
		publish(
			"correlation",
			"aggregate",
			metricFrame("correlation", "aggregate", [
				{ key: "structural_alignment", payload: [0.8] },
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		expect(
			pin(result.current.data, "correlation:structural_alignment:R01")
				?.activation,
		).toBeCloseTo(snm(0.8));
		// Region bars come from the grid frame, not from inventing pin peaks.
		expect(result.current.regions.find((r) => r.id === 1)?.strength).toBe(0);
	});

	it("does not create pins for hawkes metrics declared unpinned on the grid", () => {
		publish(
			"grid",
			SYMBOL,
			metricFrame("grid", SYMBOL, [{ key: "regions", payload: [] }]),
		);
		publish(
			"hawkes",
			SYMBOL,
			metricFrame("hawkes", SYMBOL, [
				{ key: "conditional_intensity", payload: [1.2] },
			]),
		);
		publish(
			"correlation",
			"aggregate",
			metricFrame("correlation", "aggregate", [
				{ key: "structural_alignment", payload: [0.8] },
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		expect(pin(result.current.data, "hawkes:conditional_intensity:R00")).toBeUndefined();
		expect(
			pin(result.current.data, "correlation:structural_alignment:R01")
				?.activation,
		).toBeCloseTo(snm(0.8));
	});

	it("advances marketTick when the aggregate tick moves", () => {
		publish(
			"grid",
			SYMBOL,
			metricFrame("grid", SYMBOL, [{ key: "regions", payload: [] }]),
		);
		publishTick(1n);

		const { result } = renderHook(() => useImpulseData(SYMBOL));
		expect(result.current.marketTick).toBe(1);

		publishTick(2n);

		expect(result.current.marketTick).toBe(2);
	});
});
