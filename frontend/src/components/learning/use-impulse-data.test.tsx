// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { clockAtom, signals } from "#/collections/app";
import { mergeWorkspaceAggregate, Value } from "#/types/value";
import { useImpulseData } from "./use-impulse-data";

const SYMBOL = "BTC/USD";

const frame = (
	layers: Array<{ key: string; payload: unknown }>,
	tick = 1n,
): Value =>
	new Value({
		origin: "workspace",
		role: "tick",
		scope: "aggregate",
		prefix: "workspace/tick/aggregate",
		tick,
		index: layers.map((layer) => `workspace/tick/aggregate/${layer.key}`),
		layers: layers.map((layer) =>
			new TextEncoder().encode(JSON.stringify(layer.payload)),
		),
	});

const publish = (value: Value) => {
	act(() => {
		signals.workspace.setState((prev) => ({
			...prev,
			aggregate: mergeWorkspaceAggregate(prev.aggregate, value),
		}));
	});
};

const snm = (raw: number) => Math.abs(raw) / (1 + Math.abs(raw));

const pin = (data: ReturnType<typeof useImpulseData>["data"], id: string) =>
	data.find((node) => node.id === id);

afterEach(() => {
	cleanup();
	signals.workspace.setState(() => ({}));
	signals.training.setState(() => ({}));
	clockAtom.set(null);
});

describe("useImpulseData", () => {
	it("keeps a pin when a later tick does not name it", () => {
		publish(
			frame([
				{
					key: "correlation/aggregate/structural_alignment",
					payload: [0.8],
				},
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		expect(
			pin(result.current.data, "correlation:structural_alignment:R01")
				?.activation,
		).toBeCloseTo(snm(0.8));

		publish(
			frame(
				[
					{
						key: "pumpdump/BTC/USD/blowoff_dip",
						payload: [0.4],
					},
				],
				2n,
			),
		);

		expect(
			pin(result.current.data, "correlation:structural_alignment:R01")
				?.activation,
		).toBeCloseTo(snm(0.8));
		expect(
			pin(result.current.data, "pumpdump:blowoff_dip:R08")?.activation,
		).toBeCloseTo(snm(0.4));
	});

	it("writes a pin to zero when the tick carries zero", () => {
		publish(
			frame([
				{
					key: "correlation/aggregate/structural_alignment",
					payload: [0.8],
				},
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		publish(
			frame([
				{
					key: "correlation/aggregate/structural_alignment",
					payload: [0],
				},
			]),
		);

		expect(
			pin(result.current.data, "correlation:structural_alignment:R01")
				?.activation,
		).toBe(0);
	});

	it("zeros a region when the next tick's lit set omits it", () => {
		publish(
			frame([
				{
					key: "train/regions",
					payload: [
						{
							symbol: SYMBOL,
							key: 2,
							brightness: 0.9,
							counts: 4,
							deformation: 0,
						},
					],
				},
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		expect(result.current.regions[2]?.strength).toBe(0.9);
		expect(result.current.regions[2]?.members).toBe(4);
		expect(result.current.activeEvents).toHaveLength(15);
		expect(result.current.activeEvents[2]?.activity).toBe(0.9);

		publish(
			frame([
				{
					key: "train/regions",
					payload: [
						{
							symbol: SYMBOL,
							key: 4,
							brightness: 0.6,
							counts: 2,
							deformation: 0,
						},
					],
				},
			]),
		);

		expect(result.current.regions[2]?.strength).toBe(0);
		expect(result.current.regions[4]?.strength).toBe(0.6);
	});

	it("merges aggregate kernel readings into region strength for the focus symbol", () => {
		publish(
			frame([
				{
					key: "correlation/aggregate/structural_alignment",
					payload: [0.8],
				},
				{
					key: "hawkes/aggregate/board_cascade",
					payload: [0.7],
				},
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		expect(result.current.regions[1]?.strength).toBeGreaterThan(0);
		expect(result.current.regions[10]?.strength).toBeGreaterThan(0);
	});

	it("does not create pins for hawkes metrics declared unpinned on the grid", () => {
		publish(
			frame([
				{
					key: "hawkes/BTC/USD/background_rate",
					payload: [0.62],
				},
				{
					key: "hawkes/BTC/USD/supercritical_cascade",
					payload: [1.2],
				},
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		expect(
			pin(result.current.data, "hawkes:background_rate:R07"),
		).toBeUndefined();
		expect(
			pin(result.current.data, "hawkes:supercritical_cascade:R06")?.activation,
		).toBeCloseTo(snm(1.2));
	});

	it("advances marketTick when the aggregate tick moves but held readings stay the same", () => {
		const lit = frame(
			[
				{
					key: "train/regions",
					payload: [
						{
							symbol: SYMBOL,
							key: 2,
							brightness: 0.5,
							counts: 2,
							deformation: 0,
						},
					],
				},
			],
			10n,
		);

		publish(lit);

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		expect(result.current.regions[2]?.strength).toBe(0.5);
		expect(result.current.marketTick).toBe(10);

		publish(
			frame(
				[
					{
						key: "train/regions",
						payload: [
							{
								symbol: SYMBOL,
								key: 2,
								brightness: 0.5,
								counts: 2,
								deformation: 0,
							},
						],
					},
				],
				11n,
			),
		);

		expect(result.current.regions[2]?.strength).toBe(0.5);
		expect(result.current.marketTick).toBe(11);
	});
});
