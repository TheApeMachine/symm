// @vitest-environment jsdom
import { act, render } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import { clockAtom, focusAtom, signals } from "#/collections/app";
import { Value } from "#/types/value";
import { hawkesSample, XrayHawkesPanel } from "./xray-hawkes";

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

const hawkesReading = (fields: Record<string, number | string>): Value => {
	const prefix = "test/hawkes";
	const keys = Object.keys(fields);

	return new Value({
		timestamp: 1_000_000_000n,
		prefix,
		index: keys.map((key) => `${prefix}/${key}`),
		layers: keys.map((key) => {
			const value = fields[key];
			const payload = typeof value === "string" ? value : [value];
			return new TextEncoder().encode(JSON.stringify(payload));
		}),
	});
};

describe("hawkesSample", () => {
	const row = (side: string) =>
		hawkesReading({
			side,
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
		});

	it("uses the event mark even when retained window counts stay constant", () => {
		expect(hawkesSample(row("buy"))?.postArrival).toBe(2.5);
		expect(hawkesSample(row("sell"))?.postArrival).toBeCloseTo(2.8);
	});

	it("does not draw a declared but unfitted zero-valued model", () => {
		expect(
			hawkesSample(
				hawkesReading({
					side: "buy",
					conditional_intensity: 2,
					background_rate: 0.2,
					excitation_decay: 0,
					"excitation_amplitude:buy_from_buy": 0.4,
					"excitation_amplitude:sell_from_buy": 0.1,
				}),
			),
		).toBeNull();
	});

	it("reports a malformed fitted event instead of guessing its mark", () => {
		expect(() => hawkesSample(row("unknown"))).toThrow("buy/sell mark");
	});

	it("keeps a short-gap velocity out of the spike height", () => {
		const sample = hawkesSample(
			hawkesReading({
				"arrival_rate:buy": 2,
				"arrival_rate:sell": 0.1,
				branching_spectral_radius: 0.22,
				conditional_intensity: 2.42,
				conditional_intensity_velocity: 480,
			}),
		);

		expect(sample?.intensity).toBeCloseTo(2.42);
		expect(sample?.postArrival).toBeCloseTo(2.42);
	});
});

const hawkesAggregate = (
	layers: Array<{ key: string; payload: unknown }>,
	init: { timestamp: bigint; tick: bigint },
): Value =>
	new Value({
		origin: "workspace",
		role: "tick",
		scope: "aggregate",
		prefix: "workspace/tick/aggregate",
		timestamp: init.timestamp,
		tick: init.tick,
		index: layers.map((layer) => `workspace/tick/aggregate/${layer.key}`),
		layers: layers.map((layer) =>
			new TextEncoder().encode(JSON.stringify(layer.payload)),
		),
	});

const publishAggregate = (value: Value) => {
	act(() => {
		signals.workspace.setState((prev) => ({ ...prev, aggregate: value }));
	});
};

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
			.mockReturnValue(context as unknown as GPUCanvasContext);
		const width = vi
			.spyOn(HTMLCanvasElement.prototype, "clientWidth", "get")
			.mockReturnValue(800);
		const height = vi
			.spyOn(HTMLCanvasElement.prototype, "clientHeight", "get")
			.mockReturnValue(240);
		const hawkesLayers = [
			{
				key: `hawkes/${focusAtom.get()}/conditional_intensity`,
				payload: [0.2],
			},
			{
				key: `hawkes/${focusAtom.get()}/arrival_rate:buy`,
				payload: [0.2],
			},
			{
				key: `hawkes/${focusAtom.get()}/arrival_rate:sell`,
				payload: [0.01],
			},
			{
				key: `hawkes/${focusAtom.get()}/branching_spectral_radius`,
				payload: [0.22],
			},
		];
		publishAggregate(
			hawkesAggregate(hawkesLayers, {
				timestamp: 1_000_000_000n,
				tick: 1n,
			}),
		);
		const view = render(<XrayHawkesPanel />);

		try {
			act(() => repaint?.(0));
			expect(
				view.container.querySelector('[data-f="lambda"]')?.textContent,
			).toBe("0.2000 /s");
			act(() => clockAtom.set(1500));
			act(() => repaint?.(1));
			expect(
				view.container.querySelector('[data-f="lambda"]')?.textContent,
			).toBe("0.1882 /s");
			const baselineY = context.moveTo.mock.calls[0][1];
			context.moveTo.mockClear();
			act(() => clockAtom.set(5500));
			act(() => repaint?.(2));
			expect(context.moveTo.mock.calls[0][1]).toBe(baselineY);
			expect(baselineY).toBeGreaterThan(30);
			publishAggregate(
				hawkesAggregate(hawkesLayers, {
					timestamp: 5_500_000_000n,
					tick: 2n,
				}),
			);
			act(() => repaint?.(3));
			expect(
				view.container.querySelector('[data-f="lambda"]')?.textContent,
			).toBe("0.2000 /s");

			act(() => clockAtom.set(1200));
			act(() => repaint?.(2));
			expect(
				view.container.querySelector('[data-f="lambda"]')?.textContent,
			).toBe("0.2000 /s");
		} finally {
			view.unmount();
			signals.workspace.setState(() => ({}));
			canvas.mockRestore();
			width.mockRestore();
			height.mockRestore();
			vi.unstubAllGlobals();
		}
	});
});
