// @vitest-environment jsdom
import { act, render } from "@testing-library/react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import {
	clockAtom,
	DEFAULT_FOCUS_SYMBOL,
	focusAtom,
	RingBuffer,
	signals,
} from "#/collections/app";
import type { WireMeasurement } from "#/types/capnp/measurement";
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

describe("hawkesSample", () => {
	const row = (side: string): WireMeasurement => ({
		id: "m-h",
		source: "hawkes",
		symbol: DEFAULT_FOCUS_SYMBOL,
		tick: 1n,
		at: 1_000_000_000n,
		timestamp: 1_000_000_000n,
		entity: 1,
		snr: 1.0,
		maturity: 1.0,
		separation: 0,
		provenance: [{ name: "side", value: side }],
		metadata: { side },
		metrics: Object.entries({
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
		}).map(([name, raw]) => ({ name, raw, normalized: raw })),
	});

	it("sums post-arrival intensity from the matching arrival cross-kernel", () => {
		const sample = hawkesSample(row("buy"));

		expect(sample).not.toBeNull();
		expect(sample?.at).toBe(1_000_000_000n);
		expect(sample?.intensity).toBe(2);
		expect(sample?.baseline).toBe(0.2);
		expect(sample?.decay).toBe(1);
		// 2 + 0.4 (buy) + 0.1 (sell)
		expect(sample?.postArrival).toBeCloseTo(2.5);
	});

	it("reads arrival cross-kernel terms for a sell", () => {
		const sample = hawkesSample(row("sell"));

		expect(sample).not.toBeNull();
		// 2 + 0.2 (buy) + 0.6 (sell)
		expect(sample?.postArrival).toBeCloseTo(2.8);
	});

	it("rejects an arrival whose provenance names no side", () => {
		const r = row("buy");
		r.provenance = [];
		expect(() => hawkesSample(r)).toThrow(/buy\/sell/);
	});

	it("returns null when the decay parameter has not been fitted", () => {
		const r = row("buy");
		r.metrics = r.metrics.filter((m) => m.name !== "excitation_decay");
		expect(hawkesSample(r)).toBeNull();
	});
});

describe("XrayHawkesPanel market clock", () => {
	it("decays and scrolls on clock updates without a new Hawkes measurement", () => {
		let repaint: ((time: number) => void) | null = null;
		vi.spyOn(window, "requestAnimationFrame").mockImplementation(
			(callback: FrameRequestCallback) => {
				repaint = callback;
				return 1;
			},
		);
		const context = {
			canvas: { width: 800, height: 240 },
			clearRect: vi.fn(),
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
		vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(
			context as unknown as CanvasRenderingContext2D,
		);
		vi.spyOn(HTMLCanvasElement.prototype, "clientWidth", "get").mockReturnValue(
			800,
		);
		vi.spyOn(
			HTMLCanvasElement.prototype,
			"clientHeight",
			"get",
		).mockReturnValue(240);

		const symbol = focusAtom.get() || DEFAULT_FOCUS_SYMBOL;
		const store = signals["hawkes" as keyof typeof signals] || signals.cvd;
		const ring = new RingBuffer<WireMeasurement>(50);
		ring.add({
			id: "m-hawkes-test",
			source: "hawkes",
			symbol,
			tick: 1n,
			at: 1_000_000_000n,
			timestamp: 1_000_000_000n,
			entity: 1,
			snr: 1.0,
			maturity: 1.0,
			separation: 0,
			provenance: [{ name: "side", value: "buy" }],
			metadata: { side: "buy" },
			metrics: Object.entries({
				conditional_intensity: 0.2,
				background_rate: 0.2,
				excitation_decay: 1,
				"excitation_amplitude:buy_from_buy": 0.4,
				"excitation_amplitude:sell_from_buy": 0.1,
			}).map(([name, raw]) => ({ name, raw, normalized: raw })),
		});
		store.state[symbol] = ring;
		store.state[DEFAULT_FOCUS_SYMBOL] = ring;
		store.state[""] = ring;

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
			expect(store.state[symbol]?.getBufferLength() ?? 0).toBe(1);
			const baselineY = context.moveTo.mock.calls[0][1];
			context.moveTo.mockClear();
			act(() => clockAtom.set(5500));
			act(() => repaint?.(2));
			expect(context.moveTo.mock.calls[0][1]).toBe(baselineY);
			expect(baselineY).toBeGreaterThan(30);
		} finally {
			view.unmount();
		}
	});
});
