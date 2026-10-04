import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it } from "vitest";
import {
	signals,
	tickCountAtom,
	RingBuffer,
} from "#/collections/app";
import { TerminalTopBar } from "./terminal-top-bar";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";

describe("TerminalTopBar", () => {
	beforeEach(() => {
		tickCountAtom.set(0);
		signals.training.setState(() => ({}));
	});

	it("renders both Observations and Ticks readouts", () => {
		const html = renderToStaticMarkup(<TerminalTopBar />);
		expect(html).toContain("Observations");
		expect(html).toContain("Ticks");
		expect(html).toContain('data-observations="true"');
		expect(html).toContain('data-tick="true"');
	});

	it("renders updated tick count", () => {
		tickCountAtom.set(45678);
		const html = renderToStaticMarkup(<TerminalTopBar />);
		expect(html).toContain("45,678");
	});

	it("renders training steps as ticks when available", () => {
		const ring = new RingBuffer<MeasurementT>(50);
		const measurement = new MeasurementT();
		measurement.metrics = [new MetricT("steps", 1234)];
		ring.add(measurement);

		signals.training.setState(() => ({ "BTC/USD": ring }));

		const html = renderToStaticMarkup(<TerminalTopBar />);
		expect(html).toContain("1,234");
	});
});
