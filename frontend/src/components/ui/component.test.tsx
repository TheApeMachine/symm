// @vitest-environment jsdom
import { render } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { RingBuffer } from "#/collections/ring";
import { Badge, Meter, Typography } from "#/components/ui";
import { Component } from "#/components/ui/component";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { NamedNumberT } from "#/providers/telemetry/telemetry/named-number";

describe("Component telemetry wrapper", () => {
	it("dynamically binds metrics and fields from MeasurementT to matching data attributes", () => {
		const ring = new RingBuffer<MeasurementT>(10);

		// Pre-populate with initial measurement
		const initialMeasurement = new MeasurementT();
		initialMeasurement.snr = 2.4567;
		initialMeasurement.maturity = 0.85;
		initialMeasurement.metrics = [
			new MetricT("volatility", 0.0425, 0, false, null, 0n, 0n, 0),
		];
		initialMeasurement.metadata = [new NamedNumberT("custom_meta", 99.5)];
		ring.add(initialMeasurement);

		const { container } = render(
			<Component ringBuffer={ring}>
				<div data-testid="row">
					<Badge
						data-metric-badge="status"
						label="Standby"
						variant="disabled"
					/>
					<Meter data-metric-meter="maturity" percent={0} variant="warning" />
					<Typography.Mono data-metric="snr">--</Typography.Mono>
					<Typography.Mono data-metric="volatility">--</Typography.Mono>
					<Typography.Mono data-metric="custom_meta">--</Typography.Mono>
				</div>
			</Component>,
		);

		const snrEl = container.querySelector('[data-metric="snr"]') as HTMLElement;
		const volEl = container.querySelector(
			'[data-metric="volatility"]',
		) as HTMLElement;
		const metaEl = container.querySelector(
			'[data-metric="custom_meta"]',
		) as HTMLElement;

		// Check dynamic binding
		expect(snrEl.textContent).toBe("2.4567");
		expect(volEl.textContent).toBe("0.0425");
		expect(metaEl.textContent).toBe("99.5000");

		const badgeEl = container.querySelector(
			'[data-metric-badge="status"]',
		) as HTMLElement;
		expect(badgeEl.textContent).toBe("HEALTHY");
	});

	it("binds metadata, provenance, and top-level scalar telemetry", () => {
		const ring = new RingBuffer<MeasurementT>(10);
		const measurement = new MeasurementT();

		measurement.source = "kernel-engine";
		measurement.symbol = "BTC/EUR";
		measurement.metadata = [new NamedNumberT("custom_meta", 42.12)];
		measurement.provenance = [
			{
				name: "node_id",
				value: "kernel-alpha",
				pack: () => 0,
			} as unknown as import("#/providers/telemetry/telemetry/named-string").NamedStringT,
		];
		ring.add(measurement);

		const { container } = render(
			<Component ringBuffer={ring}>
				<div>
					<Typography.Mono data-metric="source">--</Typography.Mono>
					<Typography.Mono data-metric="symbol">--</Typography.Mono>
					<Typography.Mono data-metric="custom_meta">--</Typography.Mono>
					<Typography.Mono data-metric="node_id">--</Typography.Mono>
				</div>
			</Component>,
		);

		expect(container.querySelector('[data-metric="source"]')?.textContent).toBe(
			"kernel-engine",
		);
		expect(container.querySelector('[data-metric="symbol"]')?.textContent).toBe(
			"BTC/EUR",
		);
		expect(
			container.querySelector('[data-metric="custom_meta"]')?.textContent,
		).toBe("42.1200");
		expect(
			container.querySelector('[data-metric="node_id"]')?.textContent,
		).toBe("kernel-alpha");
	});
});
