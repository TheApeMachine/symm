// @vitest-environment jsdom
import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { clockAtom, signals } from "#/collections/app";
import { Value } from "#/types/value";
import { useImpulseData } from "./use-impulse-data";

const SYMBOL = "BTC/USD";

const frame = (layers: Array<{ key: string; payload: unknown }>): Value =>
	new Value({
		origin: "workspace",
		role: "tick",
		scope: "aggregate",
		prefix: "workspace/tick/aggregate",
		index: layers.map((layer) => `workspace/tick/aggregate/${layer.key}`),
		layers: layers.map((layer) =>
			new TextEncoder().encode(JSON.stringify(layer.payload)),
		),
	});

const publish = (value: Value) => {
	act(() => {
		signals.workspace.setState((prev) => ({
			...prev,
			aggregate: value,
		}));
	});
};

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
					key: "cvd/BTC/USD/midpoint_log_return",
					payload: [0.8],
				},
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		expect(
			pin(result.current.data, "cvd:midpoint_log_return:R02")?.activation,
		).toBe(0.8);

		publish(
			frame([
				{
					key: "hawkes/BTC/USD/arrival_rate:buy",
					payload: [0.4],
				},
			]),
		);

		expect(
			pin(result.current.data, "cvd:midpoint_log_return:R02")?.activation,
		).toBe(0.8);
		expect(
			pin(result.current.data, "hawkes:arrival_rate:buy:R00")?.activation,
		).toBe(0.4);
	});

	it("writes a pin to zero when the tick carries zero", () => {
		publish(
			frame([
				{
					key: "cvd/BTC/USD/midpoint_log_return",
					payload: [0.8],
				},
			]),
		);

		const { result } = renderHook(() => useImpulseData(SYMBOL));

		publish(
			frame([
				{
					key: "cvd/BTC/USD/midpoint_log_return",
					payload: [0],
				},
			]),
		);

		expect(
			pin(result.current.data, "cvd:midpoint_log_return:R02")?.activation,
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
});
