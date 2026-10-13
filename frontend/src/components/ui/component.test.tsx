// @vitest-environment jsdom
import { render, act, cleanup } from "@testing-library/react";
import { describe, expect, it, afterEach } from "vitest";
import { createStore } from "@tanstack/store";
import { RingBuffer } from "#/collections/ring";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { NamedNumberT } from "#/providers/telemetry/telemetry/named-number";
import { NamedStringT } from "#/providers/telemetry/telemetry/named-string";
import { Component } from "./component";
import { Typography } from "./typography";

describe("Component telemetry binding", () => {
	afterEach(() => {
		cleanup();
	});

	const createSampleMeasurement = (symbol = "BTC/USD") => {
		const m = new MeasurementT();
		m.symbol = symbol;
		m.at = 1_700_000_000_000_000_000n; // valid timestamp
		m.maturity = 0.85;
		m.metrics = [
			new MetricT("contrast", 1.2346, 0.65),
			new MetricT("stability", 0.942, 0.942),
			new MetricT("ambiguity", 0.8, 0.8),
		];
		m.metadata = [new NamedNumberT("epoch", 12)];
		m.provenance = [new NamedStringT("regime", "TREND")];
		return m;
	};

	it("binds direct measurement prop into DOM elements", () => {
		const m = createSampleMeasurement();

		const view = render(
			<Component measurement={m}>
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

	it("resets DOM elements when measurement is null", () => {
		const m = createSampleMeasurement();

		const view = render(
			<Component measurement={m}>
				<Typography.Span data-metric="symbol" data-testid="symbol">
					—
				</Typography.Span>
			</Component>,
		);

		expect(view.getByTestId("symbol").textContent).toBe("BTC/USD");

		view.rerender(
			<Component measurement={null}>
				<Typography.Span data-metric="symbol" data-testid="symbol">
					—
				</Typography.Span>
			</Component>,
		);

		expect(view.getByTestId("symbol").textContent).toBe("—");
	});

	it("subscribes to a store with symbol and streams live ring updates", () => {
		const store = createStore<Record<string, RingBuffer<MeasurementT>>>({});
		const ring = new RingBuffer<MeasurementT>(10);
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
			const m = createSampleMeasurement("ETH/USD");
			ring.add(m);
			store.setState((prev) => ({ ...prev, "ETH/USD": ring }));
		});

		expect(view.getByTestId("symbol").textContent).toBe("ETH/USD");
		expect(view.getByTestId("contrast").textContent).toBe("1.23");
	});
});
