// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { RingBuffer, signals } from "#/collections/app";
import { MeasurementT } from "#/providers/telemetry/telemetry/measurement";
import { MetricT } from "#/providers/telemetry/telemetry/metric";
import { NamedNumberT } from "#/providers/telemetry/telemetry/named-number";
import { NamedStringT } from "#/providers/telemetry/telemetry/named-string";
import { ForwardLearningViz } from "./forward-learning-viz";

afterEach(() => {
	cleanup();
	signals.training.setState(() => ({}));
	vi.useRealTimers();
	vi.unstubAllGlobals();
});

it("starts playback when a completed episode arrives after an empty queue", async () => {
	vi.useFakeTimers();
	vi.stubGlobal(
		"fetch",
		vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({ branches: [] }),
		}),
	);
	const { container } = render(<ForwardLearningViz />);
	expect(container.textContent).toContain("Awaiting historical replay");
	const ring = new RingBuffer<MeasurementT>(8);

	await act(async () => {
		for (const [index, price] of [100, 102, 101].entries()) {
			const frame = new MeasurementT();
			frame.source = "training";
			frame.symbol = "BTC/USD";
			frame.tick = BigInt(index + 1);
			frame.metrics = [
				new MetricT("price", price),
				new MetricT("stage_code", 1),
			];
			frame.metadata = [new NamedNumberT("excursion_start", 1)];
			if (index === 2)
				frame.provenance = [new NamedStringT("excursion_event", "completed")];
			ring.add(frame);
		}
		signals.training.setState(() => ({ "BTC/USD": ring }));
	});
	expect(container.textContent).toContain("1 frames evaluated");
	await act(async () => {
		vi.advanceTimersByTime(16);
	});
	expect(container.textContent).toContain("3 frames evaluated");
});

it("renders screen plate scanlines and time-scale span for tape fragments", async () => {
	vi.useFakeTimers();
	vi.stubGlobal(
		"fetch",
		vi.fn().mockResolvedValue({
			ok: true,
			json: async () => ({ branches: [] }),
		}),
	);
	const { container } = render(<ForwardLearningViz />);
	const ring = new RingBuffer<MeasurementT>(8);

	// Start at 14:00:00 UTC (1718028000000 ms = 1718028000000000000 ns)
	const baseNanos = 1718028000000000000n;
	const minuteNanos = 60_000_000_000n;

	await act(async () => {
		for (const [index, price] of [100, 102, 105].entries()) {
			const frame = new MeasurementT();
			frame.source = "training";
			frame.symbol = "BTC/USD";
			frame.tick = BigInt(index + 1);
			frame.at = baseNanos + BigInt(index * 2) * minuteNanos; // spans 4 minutes
			frame.metrics = [
				new MetricT("price", price),
				new MetricT("stage_code", 1),
			];
			ring.add(frame);
		}
		signals.training.setState(() => ({ "BTC/USD": ring }));
	});

	// Check time-scale span badge in header (4m)
	expect(container.textContent).toContain("4m");
	// Check screen scanlines present
	const scanlineEl = container.querySelector("[aria-hidden='true']");
	expect(scanlineEl).toBeDefined();
});

it("renders A/B/C and ENTER/EXIT markers from a trained fragment", async () => {
	vi.useFakeTimers();
	vi.stubGlobal(
		"fetch",
		vi.fn().mockImplementation(async (url: string) => {
			if (String(url).includes("/training/fragments")) {
				return {
					ok: true,
					json: async () => [
						{
							id: 1,
							symbol: "BTC/USD",
							epoch: 100,
							mark_a: 7,
							mark_b: 10,
							mark_c: 15,
							entry_price: 60000,
							exit_price: 63000,
							magnitude: 0.05,
							direction: "up",
							class: "up",
							tokens: ["R0", "R1", "R0"],
							points: [
								{ x: 0, y: 60000, seq: 7, time: 1 },
								{ x: 1, y: 60100, seq: 8, time: 2 },
								{ x: 2, y: 60200, seq: 9, time: 3 },
								{ x: 3, y: 60500, seq: 10, time: 4 },
								{ x: 4, y: 61000, seq: 12, time: 5 },
								{ x: 5, y: 62000, seq: 14, time: 6 },
								{ x: 6, y: 63000, seq: 15, time: 7 },
							],
							entry_idx: 2,
							exit_idx: 5,
							predicted_entry_idx: 2,
							predicted_exit_idx: 5,
							learned_at: new Date().toISOString(),
						},
					],
				};
			}
			return { ok: true, json: async () => ({ branches: [] }) };
		}),
	);

	const { container } = render(<ForwardLearningViz tapeSource="historical" />);

	await act(async () => {
		vi.advanceTimersByTime(50);
		await Promise.resolve();
	});

	expect(container.textContent).toContain("A:");
	expect(container.textContent).toContain("B:");
	expect(container.textContent).toContain("C:");
	expect(container.textContent).toContain("ENTER");
	expect(container.textContent).toContain("EXIT");
	expect(container.textContent).toContain("PREDICTED ENTER");
	expect(container.textContent).toContain("PREDICTED EXIT");
	expect(container.textContent).toContain("UPWARD EXCURSION");
	expect(container.textContent).toContain("[up]");
	expect(
		container.querySelector('[data-l="fragment-class"]')?.textContent,
	).toContain("up");
	expect(
		container.querySelector('[data-l="fragment-class-detail"]')?.textContent,
	).toContain("up");
});

it("surfaces up_friction class on fragment list and detail", async () => {
	vi.useFakeTimers();
	vi.stubGlobal(
		"fetch",
		vi.fn().mockImplementation(async (url: string) => {
			if (String(url).includes("/training/fragments")) {
				return {
					ok: true,
					json: async () => [
						{
							id: 2,
							symbol: "ETH/USD",
							epoch: 101,
							mark_a: 1,
							mark_b: 2,
							mark_c: 3,
							entry_price: 3000,
							exit_price: 3010,
							magnitude: 0.003,
							direction: "up",
							class: "up_friction",
							tokens: ["R0"],
							points: [
								{ x: 0, y: 3000, seq: 1, time: 1 },
								{ x: 1, y: 3005, seq: 2, time: 2 },
								{ x: 2, y: 3010, seq: 3, time: 3 },
							],
							entry_idx: 0,
							exit_idx: 2,
							predicted_entry_idx: -1,
							predicted_exit_idx: -1,
							learned_at: new Date().toISOString(),
						},
					],
				};
			}
			return { ok: true, json: async () => ({ branches: [] }) };
		}),
	);

	const { container } = render(<ForwardLearningViz tapeSource="historical" />);

	await act(async () => {
		vi.advanceTimersByTime(50);
		await Promise.resolve();
	});

	expect(container.textContent).toContain("UP FRICTION");
	expect(container.textContent).toContain("[up_friction]");
	expect(
		container.querySelector('[data-l="fragment-class-detail"]')?.textContent,
	).toContain("up_friction");
});

it("keeps a pinned historical fragment when live BTC hub updates arrive", async () => {
	vi.useFakeTimers();
	vi.stubGlobal(
		"fetch",
		vi.fn().mockImplementation(async (url: string) => {
			if (String(url).includes("/training/fragments")) {
				return {
					ok: true,
					json: async () => [
						{
							id: 9,
							symbol: "ETH/USD",
							epoch: 42,
							mark_a: 1,
							mark_b: 2,
							mark_c: 3,
							entry_price: 3000,
							exit_price: 3100,
							magnitude: 0.033,
							direction: "up",
							class: "up",
							tokens: ["R0", "R1"],
							points: [
								{ x: 0, y: 3000, seq: 1, time: 1 },
								{ x: 1, y: 3050, seq: 2, time: 2 },
								{ x: 2, y: 3100, seq: 3, time: 3 },
							],
							entry_idx: 0,
							exit_idx: 2,
							predicted_entry_idx: -1,
							predicted_exit_idx: -1,
							learned_at: new Date().toISOString(),
						},
					],
				};
			}
			return { ok: true, json: async () => ({ branches: [] }) };
		}),
	);

	const { container } = render(<ForwardLearningViz tapeSource="historical" />);

	await act(async () => {
		vi.advanceTimersByTime(50);
		await Promise.resolve();
	});

	expect(container.textContent).toContain("ETH/USD");
	expect(container.textContent).toContain("3 frames evaluated");
	expect(
		container.querySelector('[data-l="historical-run-active"]'),
	).toBeTruthy();

	const ring = new RingBuffer<MeasurementT>(8);
	await act(async () => {
		const frame = new MeasurementT();
		frame.source = "training";
		frame.symbol = "BTC/USD";
		frame.tick = 99n;
		frame.metrics = [new MetricT("price", 85000), new MetricT("stage_code", 1)];
		ring.add(frame);
		signals.training.setState(() => ({ "BTC/USD": ring }));
	});

	// Selection must stick: live BTC must not replace the ETH fragment tape.
	expect(container.textContent).toContain("ETH/USD");
	expect(container.textContent).toContain("3 frames evaluated");
	expect(container.textContent).not.toContain("85000");
	expect(
		container.querySelector('[data-l="tape-title"]')?.textContent,
	).toContain("HISTORICAL RUNS");
});

it("scales excursions across the full canvas width and renders relative timeline and boundaries", async () => {
	vi.useFakeTimers();
	vi.stubGlobal(
		"fetch",
		vi.fn().mockImplementation(async (url: string) => {
			if (String(url).includes("/training/fragments")) {
				return {
					ok: true,
					json: async () => [
						{
							id: 42,
							symbol: "LIGHTER/USD",
							epoch: 1,
							mark_a: 271,
							mark_b: 275,
							mark_c: 277,
							entry_price: 3.091,
							exit_price: 3.098,
							magnitude: 0.002,
							direction: "up",
							class: "flat",
							tokens: ["R0", "R1"],
							points: [
								{ x: 0, y: 3.091, seq: 271, time: 1718028000000 },
								{ x: 1, y: 3.093, seq: 273, time: 1718028000090 },
								{ x: 2, y: 3.095, seq: 275, time: 1718028000180 },
								{ x: 3, y: 3.097, seq: 276, time: 1718028000270 },
								{ x: 4, y: 3.098, seq: 277, time: 1718028000350 },
							],
							entry_idx: 0,
							exit_idx: 4,
							predicted_entry_idx: -1,
							predicted_exit_idx: -1,
							learned_at: new Date().toISOString(),
						},
					],
				};
			}
			return { ok: true, json: async () => ({ branches: [] }) };
		}),
	);

	const { container } = render(<ForwardLearningViz tapeSource="historical" />);

	await act(async () => {
		vi.advanceTimersByTime(50);
		await Promise.resolve();
	});

	// Check boundary display with relative tick steps (+4, +2)
	expect(container.textContent).toContain("A: 271");
	expect(container.textContent).toContain("B: 275");
	expect(container.textContent).toContain("(+4)");
	expect(container.textContent).toContain("C: 277");
	expect(container.textContent).toContain("(+2)");

	// Check relative timeline ticks (t0 anchor and relative offsets)
	expect(container.textContent).toContain("t₀");
	expect(container.textContent).toContain("+0.09s");
	expect(container.textContent).toContain("+0.35s");
});

it("replays fragment according to actual tape duration and timestamps", async () => {
	vi.useFakeTimers();
	vi.stubGlobal(
		"fetch",
		vi.fn().mockImplementation(async (url: string) => {
			if (String(url).includes("/training/fragments")) {
				return {
					ok: true,
					json: async () => [
						{
							id: 101,
							symbol: "GNOT/USD",
							epoch: 1,
							mark_a: 723,
							mark_b: 738,
							mark_c: 739,
							entry_price: 0.07381,
							exit_price: 0.07382,
							magnitude: 0.001,
							direction: "up",
							class: "flat",
							tokens: ["R08", "R10"],
							points: [
								{ x: 0, y: 0.07381, seq: 723, time: 1718028000000 },
								{ x: 1, y: 0.07382, seq: 738, time: 1718028000500 },
								{ x: 2, y: 0.07382, seq: 739, time: 1718028000940 },
							],
							entry_idx: 0,
							exit_idx: 2,
							predicted_entry_idx: -1,
							predicted_exit_idx: -1,
							learned_at: new Date().toISOString(),
						},
					],
				};
			}
			return { ok: true, json: async () => ({ branches: [] }) };
		}),
	);

	const { container } = render(<ForwardLearningViz tapeSource="historical" />);

	await act(async () => {
		vi.advanceTimersByTime(50);
		await Promise.resolve();
	});

	// Replay button should be enabled for auto-selected fragment
	const replayBtn = container.querySelector(
		"button[title='Replay this fragment animation']",
	) as HTMLButtonElement | null;
	expect(replayBtn).toBeDefined();
	expect(replayBtn?.disabled).toBe(false);

	await act(async () => {
		replayBtn?.click();
	});

	// At start of replay (t=0)
	expect(container.textContent).toContain("1 frames evaluated");

	// Advance by 300ms (before point 1 at 500ms)
	await act(async () => {
		vi.advanceTimersByTime(300);
	});
	expect(container.textContent).toContain("1 frames evaluated");

	// Advance by another 300ms to 600ms (point 1 at 500ms reached)
	await act(async () => {
		vi.advanceTimersByTime(300);
	});
	expect(container.textContent).toContain("2 frames evaluated");

	// Advance past 940ms (point 2 reached, tape completed)
	await act(async () => {
		vi.advanceTimersByTime(400);
	});
	expect(container.textContent).toContain("3 frames evaluated");
});

