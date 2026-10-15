// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { RingBuffer, signals } from "#/collections/app";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { OpenPositions } from "./open-positions";

const measurement = (values: Record<string, number>) => {
	const m = new MeasurementT();
	m.source = "training";
	m.symbol = "RLC/USD";
	m.metrics = Object.entries(values).map(([name, raw]) => {
		const metric = new MetricT();
		metric.name = name;
		metric.raw = raw;
		return metric;
	});
	return m;
};

afterEach(() => {
	cleanup();
	signals.training.setState(() => ({}));
});

describe("OpenPositions", () => {
	it("shows the empty state when nothing is held", () => {
		const ring = new RingBuffer<MeasurementT>(8);
		ring.add(measurement({ position_open: 0 }));
		signals.training.setState(() => ({ "RLC/USD": ring }));
		const view = render(<OpenPositions />);
		expect(view.getByText("No open positions")).toBeTruthy();
	});

	it("draws a card from the position metrics the backend sent", () => {
		const ring = new RingBuffer<MeasurementT>(8);
		signals.training.setState(() => ({ "RLC/USD": ring }));
		const view = render(<OpenPositions />);
		act(() => {
			ring.add(
				measurement({
					position_open: 1,
					position_entry: 1,
					position_bid: 1.1,
					position_pnl_pct: 0.05,
					position_exit_room: 4,
					position_held_s: 75,
				}),
			);
			signals.training.setState((s) => ({ ...s }));
		});
		expect(view.getByText("RLC/USD")).toBeTruthy();
		expect(view.getByText("held 1m 15s")).toBeTruthy();
		expect(view.getByText("4.0×")).toBeTruthy();
		expect(view.getByText("5.0%")).toBeTruthy();
		expect(view.getByText("waiting for tokens")).toBeTruthy();
	});
});
