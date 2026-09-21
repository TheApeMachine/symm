// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { focusAtom, RingBuffer, signals } from "#/collections/app";
import { LearningDevelopmentT } from "#/providers/telemetry/telemetry/learning-development";
import { LearningQuantityT } from "#/providers/telemetry/telemetry/learning-quantity";
import { LearningRegionT } from "#/providers/telemetry/telemetry/learning-region";
import type { WireMeasurement } from "#/types/capnp/measurement";
import { LearningDashboard } from "./dashboard";

afterEach(() => {
	cleanup();
	signals.training.setState(() => ({}));
});

describe("LearningDashboard", () => {
	it("renders received coordinates and basin IDs instead of grouping sources", () => {
		focusAtom.set("BTC/USD");
		const measurement: any = {
			id: "meas-learn-1",
			source: "training",
			symbol: "BTC/USD",
			tick: 1n,
			at: 1000n,
			timestamp: 1000n,
			entity: 1,
			maturity: 0.9,
			snr: 1.0,
			separation: 0,
			metrics: [],
			metadata: {},
			provenance: [],
		};
		measurement.grid = new LearningDevelopmentT();
		measurement.grid.symbol = "BTC/USD";
		const left = new LearningQuantityT();
		Object.assign(left, {
			id: 101n,
			source: "same-owner",
			label: "left",
			x: -1,
			y: 1,
			activity: 2,
			present: true,
		});
		const right = new LearningQuantityT();
		Object.assign(right, {
			id: 202n,
			source: "same-owner",
			label: "right",
			x: 1,
			y: -1,
			activity: 1,
			present: true,
		});
		const basin = new LearningRegionT();
		Object.assign(basin, { id: 101n, strength: 3, authority: 0.8, members: 2 });
		measurement.grid.quantities = [left, right];
		measurement.grid.regions = [basin];
		const ring = new RingBuffer<WireMeasurement>(4);
		ring.add(measurement);
		signals.training.setState(() => ({ "BTC/USD": ring }));
		const { container } = render(<LearningDashboard />);
		expect(
			container.querySelector<HTMLElement>('[data-metric="action"]')?.innerText,
		).toEqual("—");
		expect(
			container.querySelector<HTMLElement>('[data-metric="edge"]')?.innerText,
		).toEqual("—");
		expect(container.querySelector('[data-metric="confidence"]')).toBeNull();
		expect(
			container.querySelector('[data-l="map-meta"]')?.textContent,
		).toContain("2 numeric cells · 1 hot regions");
		const circles = container.querySelectorAll('[data-l="map-points"] circle');
		expect(circles).toHaveLength(2);
		expect(circles[0].getAttribute("cx")).not.toEqual(
			circles[1].getAttribute("cx"),
		);
		const previous = circles[0].getAttribute("cx");
		left.x = 0;
		act(() => {
			ring.add(measurement);
			signals.training.setState((state: any) => ({ ...state }));
		});
		expect(circles[0].getAttribute("cx")).not.toEqual(previous);
	});
});
