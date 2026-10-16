// @vitest-environment jsdom
import { render, act, cleanup } from "@testing-library/react";
import { describe, expect, it, afterEach } from "vitest";
import { createStore } from "@tanstack/store";
import { RingBuffer } from "#/collections/ring";
import { Value } from "#/types/value";
import { Component } from "./component";
import { Typography } from "./typography";

describe("Component telemetry binding", () => {
	afterEach(() => {
		cleanup();
	});

	const createSampleValue = (symbol = "BTC/USD") => {
		return new Value({
			scope: symbol,
			timestamp: 1_700_000_000_000_000_000n,
			metrics: [
				{ name: "contrast", raw: 1.2346, snm: 0.65 },
				{ name: "stability", raw: 0.942, snm: 0.942 },
				{ name: "ambiguity", raw: 0.8, snm: 0.8 },
			],
			metadata: [{ name: "epoch", value: 12 }],
			provenance: [{ name: "regime", value: "TREND" }],
		});
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
				<Typography.Span data-f="m:contrast" data-testid="raw-contrast">
					—
				</Typography.Span>
				<div data-mbar="contrast" data-testid="bar" style={{ width: "0%" }} />
			</Component>,
		);

		expect(view.getByTestId("symbol").textContent).toBe("BTC/USD");
		expect(view.getByTestId("contrast").textContent).toBe("1.235");
		expect(view.getByTestId("stability").textContent).toBe("94.2%");
		expect(view.getByTestId("ambiguity").textContent).toBe("true");
		expect(view.getByTestId("raw-contrast").textContent).toBe("1.2346");
		expect(view.getByTestId("bar").style.width).toBe("65%");
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

	it("subscribes to a store with symbol and streams live ring updates", () => {
		const store = createStore<Record<string, RingBuffer<Value>>>({});
		const ring = new RingBuffer<Value>(10);
		store.setState((prev) => ({ ...prev, "ETH/USD": ring }));

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
			const v = createSampleValue("ETH/USD");
			ring.add(v);
			store.setState((prev) => ({ ...prev, "ETH/USD": ring }));
		});

		expect(view.getByTestId("symbol").textContent).toBe("ETH/USD");
		expect(view.getByTestId("contrast").textContent).toBe("1.23");
	});
});
