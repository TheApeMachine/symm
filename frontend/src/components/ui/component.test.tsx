// @vitest-environment jsdom
import { render, act, cleanup } from "@testing-library/react";
import { describe, expect, it, afterEach } from "vitest";
import { createStore } from "@tanstack/store";
import { Value } from "#/types/value";
import { Component } from "./component";
import { Sparkline } from "./sparkline";
import { Typography } from "./typography";

/*
append adds one scalar layer the way the Go Append addresses it: the key
joined below the frame's prefix, the payload a JSON array (the []float64
of the Append call), one float per key.
*/
const append = (value: Value, key: string, payload: unknown) => {
	const encoded = new TextEncoder().encode(JSON.stringify(payload));

	value.layers = [...value.layers, encoded];
	value.index = [...value.index, `${value.prefix}/${key}`];
};

describe("Component telemetry binding", () => {
	afterEach(() => {
		cleanup();
	});

	const createSampleValue = (symbol = "BTC/USD") => {
		const value = new Value({
			scope: symbol,
			prefix: "workspace/tick/aggregate",
			timestamp: 1_700_000_000_000_000_000n,
		});

		append(value, "contrast", [1.2346]);
		append(value, "stability", [0.942]);
		append(value, "ambiguity", [0.8]);
		append(value, "regime", ["TREND"]);

		return value;
	};

	it("binds direct value prop into DOM elements", () => {
		const v = createSampleValue();

		const view = render(
			<Component value={v}>
				<Typography.Span data-metric="symbol" data-testid="symbol">
					—
				</Typography.Span>
				<Typography.Span
					data-metric="contrast"
					data-format="fixed:3"
					data-testid="contrast"
				>
					—
				</Typography.Span>
				<Typography.Span
					data-metric="stability"
					data-format="pct"
					data-testid="stability"
				>
					—
				</Typography.Span>
				<Typography.Span
					data-metric="ambiguity"
					data-format="bool"
					data-testid="ambiguity"
				>
					—
				</Typography.Span>
			</Component>,
		);

		expect(view.getByTestId("symbol").textContent).toBe("BTC/USD");
		expect(view.getByTestId("contrast").textContent).toBe("1.235");
		expect(view.getByTestId("stability").textContent).toBe("94.2%");
		expect(view.getByTestId("ambiguity").textContent).toBe("true");
	});

	it("resets DOM elements when value is null", () => {
		const v = createSampleValue();

		const view = render(
			<Component value={v}>
				<Typography.Span data-metric="symbol" data-testid="symbol">
					—
				</Typography.Span>
			</Component>,
		);

		expect(view.getByTestId("symbol").textContent).toBe("BTC/USD");

		view.rerender(
			<Component value={null}>
				<Typography.Span data-metric="symbol" data-testid="symbol">
					—
				</Typography.Span>
			</Component>,
		);

		expect(view.getByTestId("symbol").textContent).toBe("—");
	});

	it("subscribes to a store with symbol and streams live frames", () => {
		const store = createStore<Record<string, Value>>({});

		const view = render(
			<Component store={store} symbol="ETH/USD">
				<Typography.Span data-metric="symbol" data-testid="symbol">
					—
				</Typography.Span>
				<Typography.Span
					data-metric="contrast"
					data-format="fixed:2"
					data-testid="contrast"
				>
					—
				</Typography.Span>
			</Component>,
		);

		expect(view.getByTestId("symbol").textContent).toBe("—");

		act(() => {
			store.setState((prev) => ({
				...prev,
				"ETH/USD": createSampleValue("ETH/USD"),
			}));
		});

		expect(view.getByTestId("symbol").textContent).toBe("ETH/USD");
		expect(view.getByTestId("contrast").textContent).toBe("1.23");
	});

	it("paints a sparkline from frame history for the named headline metric", () => {
		const store = createStore<Record<string, Value>>({});

		const view = render(
			<Component store={store} symbol="ETH/USD">
				<Sparkline
					data-k="sparkline"
					data-series="contrast"
					title="contrast"
				/>
			</Component>,
		);

		act(() => {
			const first = new Value({
				scope: "ETH/USD",
				prefix: "workspace/tick/aggregate",
			});
			append(first, "contrast", [0.1]);
			append(first, "stability", [9]);
			store.setState((prev) => ({ ...prev, "ETH/USD": first }));
		});

		act(() => {
			const second = new Value({
				scope: "ETH/USD",
				prefix: "workspace/tick/aggregate",
			});
			append(second, "contrast", [0.9]);
			store.setState((prev) => ({ ...prev, "ETH/USD": second }));
		});

		const spark = view.container.querySelector('polyline[fill="none"]');
		const points = spark?.getAttribute("points") ?? "";

		expect(points).not.toBe("0.0,29.0 150,29.0");
		expect(points.split(" ")).toHaveLength(2);
		const ys = points.split(" ").map((pair) => Number(pair.split(",")[1]));
		expect(ys[0]).toBeGreaterThan(ys[1]);
	});

	it("does not paint the first metric when the named series is absent", () => {
		const store = createStore<Record<string, Value>>({});

		const view = render(
			<Component store={store} symbol="ETH/USD">
				<Sparkline
					data-k="sparkline"
					data-series="missing"
					title="missing"
				/>
			</Component>,
		);

		act(() => {
			const frame = new Value({
				scope: "ETH/USD",
				prefix: "workspace/tick/aggregate",
			});
			append(frame, "contrast", [0.9]);
			store.setState((prev) => ({ ...prev, "ETH/USD": frame }));
		});

		const spark = view.container.querySelector('polyline[fill="none"]');
		expect(spark?.getAttribute("points")).toBe("0.0,29.0 150,29.0");
	});
});
