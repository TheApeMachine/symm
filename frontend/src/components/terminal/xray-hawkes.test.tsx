// @vitest-environment jsdom
import { act, render } from "@testing-library/react";
import {
	clockAtom,
	focusStore,
	getMeasurementStore,
	signals,
} from "#/collections/app";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { hawkesSample, XrayHawkesPanel } from "./xray-hawkes";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { NamedStringT } from "#/providers/telemetry/telemetry/named-string";

describe("XrayHawkesPanel", () => {
	it("renders the arrival-process readouts and canvas shell", () => {
		const markup = renderToStaticMarkup(<XrayHawkesPanel />);

		expect(markup).toContain("<canvas");
		expect(markup).toContain('data-f="events"');
		expect(markup).toContain('data-f="lambda"');
		expect(markup).toContain('data-f="mu"');
		expect(markup).toContain('data-f="sells"');
		expect(markup).toContain('data-f="eta"');
		expect(markup).toContain('class="absolute inset-0"');
	});
});

describe("hawkesSample", () => {
	const row = (side: string) => {
		const measurement = new MeasurementT();
		measurement.at = 1_000_000_000n;
		measurement.provenance = [new NamedStringT("side", side)];
		measurement.metrics = Object.entries({
			event_count: 65,
			"event_count:buy": 40,
			"event_count:sell": 25,
			conditional_intensity: 2,
			background_rate: 0.2,
			excitation_decay: 1,
			"excitation_amplitude:buy_from_buy": 0.4,
			"excitation_amplitude:sell_from_buy": 0.1,
			"excitation_amplitude:buy_from_sell": 0.2,
			"excitation_amplitude:sell_from_sell": 0.6,
		}).map(([name, value]) => new MetricT(name, value));
		return measurement;
	};

	it("uses the event mark even when retained window counts stay constant", () => {
		expect(hawkesSample(row("buy"))?.postArrival).toBe(2.5);
		expect(hawkesSample(row("sell"))?.postArrival).toBeCloseTo(2.8);
	});

	it("does not draw a declared but unfitted zero-valued model", () => {
		const measurement = row("buy");
		measurement.metrics.find(
			(metric) => metric.name === "excitation_decay",
		)!.raw = 0;
		expect(hawkesSample(measurement)).toBeNull();
	});

	it("reports a malformed fitted event instead of guessing its mark", () => {
		expect(() => hawkesSample(row("unknown"))).toThrow("buy/sell mark");
	});
});

describe("XrayHawkesPanel market clock", () => {
	it("decays and scrolls on clock updates without a new Hawkes measurement", () => {
		let repaint: FrameRequestCallback | undefined;
		vi.stubGlobal("requestAnimationFrame", (callback: FrameRequestCallback) => {
			repaint = callback;
			return 1;
		});
		vi.stubGlobal("cancelAnimationFrame", vi.fn());
		vi.stubGlobal(
			"ResizeObserver",
			class {
				observe() {}
				disconnect() {}
			},
		);
		const context = {
			scale: vi.fn(),
			setLineDash: vi.fn(),
			beginPath: vi.fn(),
			moveTo: vi.fn(),
			lineTo: vi.fn(),
			stroke: vi.fn(),
			closePath: vi.fn(),
			fill: vi.fn(),
			fillText: vi.fn(),
		};
		const canvas = vi
			.spyOn(HTMLCanvasElement.prototype, "getContext")
			.mockReturnValue(context as unknown as CanvasRenderingContext2D);
		const width = vi
			.spyOn(HTMLCanvasElement.prototype, "clientWidth", "get")
			.mockReturnValue(800);
		const height = vi
			.spyOn(HTMLCanvasElement.prototype, "clientHeight", "get")
			.mockReturnValue(240);
		const symbol = focusStore.state;
		const store = getMeasurementStore("hawkes", symbol);
		store.state.clear();
		const measurement = new MeasurementT();
		measurement.at = 1_000_000_000n;
		measurement.provenance = [new NamedStringT("side", "buy")];
		measurement.metrics = Object.entries({
			conditional_intensity: 0.2,
			background_rate: 0.2,
			excitation_decay: 1,
			"excitation_amplitude:buy_from_buy": 0.4,
			"excitation_amplitude:sell_from_buy": 0.1,
		}).map(([name, value]) => new MetricT(name, value));
		store.state.add(measurement);
		const view = render(<XrayHawkesPanel />);

		try {
			act(() => repaint?.(0));
			expect(
				view.container.querySelector('[data-f="lambda"]')?.textContent,
			).toBe("0.7000 /s");
			act(() => clockAtom.set(1500));
			act(() => repaint?.(1));
			expect(
				view.container.querySelector('[data-f="lambda"]')?.textContent,
			).toBe("0.5033 /s");
			expect(store.state.getBufferLength()).toBe(1);
			const baselineY = context.moveTo.mock.calls[0][1];
			context.moveTo.mockClear();
			act(() => clockAtom.set(5500));
			act(() => repaint?.(2));
			expect(context.moveTo.mock.calls[0][1]).toBe(baselineY);
			expect(baselineY).toBeGreaterThan(30);
			const next = new MeasurementT();
			Object.assign(next, measurement, { at: 5_500_000_000n });
			act(() => {
				store.state.add(next);
				signals.hawkes.setState((previous) => ({ ...previous }));
			});
			act(() => repaint?.(3));
			expect(
				view.container.querySelector('[data-f="lambda"]')?.textContent,
			).toBe("0.7000 /s");

			act(() => clockAtom.set(1200));
			act(() => repaint?.(2));
			expect(
				view.container.querySelector('[data-f="lambda"]')?.textContent,
			).toBe("0.7000 /s");
		} finally {
			view.unmount();
			store.state.clear();
			canvas.mockRestore();
			width.mockRestore();
			height.mockRestore();
			vi.unstubAllGlobals();
		}
	});
});
