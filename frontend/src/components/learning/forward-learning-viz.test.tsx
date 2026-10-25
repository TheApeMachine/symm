// @vitest-environment jsdom
import { act, cleanup, render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { RingBuffer, signals } from "#/collections/app";
import { type DetectionTrade, detectionsStore } from "#/collections/detections";
import { EXCURSION_KEYS } from "#/collections/hindsight";
import { sendCommand } from "#/providers/websocket";
import { Value } from "#/types/value";
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
	const ring = new RingBuffer<Value>(8);

	await act(async () => {
		for (const [index, price] of [100, 102, 101].entries()) {
			const frame = new Value({
				origin: "training",
				scope: "BTC/USD",
				tick: BigInt(index + 1),
				metrics: [
					{ name: "price", raw: price },
					{ name: "stage_code", raw: 1 },
				],
				metadata: [{ name: "excursion_start", value: 1 }],
				provenance:
					index === 2 ? [{ name: "excursion_event", value: "completed" }] : [],
			});
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
	const ring = new RingBuffer<Value>(8);

	// Start at 14:00:00 UTC (1718028000000 ms = 1718028000000000000 ns)
	const baseNanos = 1718028000000000000n;
	const minuteNanos = 60_000_000_000n;

	await act(async () => {
		for (const [index, price] of [100, 102, 105].entries()) {
			const frame = new Value({
				origin: "training",
				scope: "BTC/USD",
				tick: BigInt(index + 1),
				timestamp: baseNanos + BigInt(index * 2) * minuteNanos,
				metrics: [
					{ name: "price", raw: price },
					{ name: "stage_code", raw: 1 },
				],
			});
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

vi.mock("#/providers/websocket", () => ({ sendCommand: vi.fn() }));

afterEach(() => {
	detectionsStore.setState(() => ({
		runs: null,
		excursions: null,
		tape: null,
	}));
});

const ms = (at: number) => at * 1_000_000;

type Fixture = {
	symbol: string;
	class: string;
	fields: Partial<Record<(typeof EXCURSION_KEYS)[number], number>>;
	trades: DetectionTrade[];
};

/*
historical renders the Historical runs tab over one detected epoch holding
fixture's excursion, as the hub streams it: the epoch, its excursions, then
(once the tab opened the newest one and asked for it) its trades.
*/
const historical = async (fixture: Fixture) => {
	const run = "1792189318815706000";
	const fields = Object.fromEntries(
		EXCURSION_KEYS.map((key) => [key, 0]),
	) as Record<(typeof EXCURSION_KEYS)[number], number>;
	Object.assign(
		fields,
		{ b_price: 100, c_price: 101, c_tick: 9 },
		fixture.fields,
	);

	detectionsStore.setState(() => ({
		runs: {
			status: "done",
			error: null,
			items: [
				{
					epoch: run,
					fields: {
						first_tick: 1,
						last_tick: 9,
						ticks: 9,
						symbols: 1,
						up: 1,
						down: 0,
						chop: 0,
						flat: 0,
					},
				},
			],
		},
		excursions: {
			run,
			status: "done",
			error: null,
			items: [{ symbol: fixture.symbol, class: fixture.class, fields }],
		},
		tape: null,
	}));

	const view = render(<ForwardLearningViz tapeSource="historical" />);

	await act(async () => {
		vi.advanceTimersByTime(50);
		await Promise.resolve();
	});

	// The tab asked for the opened excursion's tape: the hub answers.
	const asked = detectionsStore.state.tape;
	expect(asked).toMatchObject({
		run,
		symbol: fixture.symbol,
		class: fixture.class,
		cTick: "9",
	});
	expect(sendCommand).toHaveBeenCalledWith({
		type: "s3",
		prefix: `detections/${fixture.class}/${fixture.symbol}/`,
		run,
		symbol: fixture.symbol,
		class: fixture.class,
		at: "9",
	});

	await act(async () => {
		detectionsStore.setState((state) => ({
			...state,
			tape: state.tape && {
				...state.tape,
				status: "done",
				items: fixture.trades,
			},
		}));
		vi.advanceTimersByTime(50);
		await Promise.resolve();
	});

	return view;
};

it("renders A/B/C and ENTER/EXIT markers from a detected excursion", async () => {
	vi.useFakeTimers();
	const { container } = await historical({
		symbol: "BTC/USD",
		class: "up",
		fields: {
			a_idx: 7,
			b_idx: 10,
			c_idx: 15,
			d_idx: 15,
			entry_idx: 9,
			exit_idx: 14,
			b_price: 60000,
			c_price: 63000,
			entry_ask: 60200,
			exit_bid: 62000,
		},
		trades: [
			{ seq: 7, price: 60000, at: ms(1) },
			{ seq: 8, price: 60100, at: ms(2) },
			{ seq: 9, price: 60200, at: ms(3) },
			{ seq: 10, price: 60500, at: ms(4) },
			{ seq: 12, price: 61000, at: ms(5) },
			{ seq: 14, price: 62000, at: ms(6) },
			{ seq: 15, price: 63000, at: ms(7) },
		],
	});

	expect(container.textContent).toContain("A:");
	expect(container.textContent).toContain("B:");
	expect(container.textContent).toContain("C:");
	expect(container.textContent).toContain("ENTER");
	expect(container.textContent).toContain("EXIT");
	// Detections carry no predicted entry or exit (train's and paper's).
	expect(container.textContent).not.toContain("PREDICTED ENTER");
	expect(container.textContent).toContain("UPWARD EXCURSION");
	expect(container.textContent).toContain("[up]");
	expect(container.textContent).toContain("+5.00%");
	expect(
		container.querySelector('[data-l="fragment-class"]')?.textContent,
	).toContain("up");
	expect(
		container.querySelector('[data-l="fragment-class-detail"]')?.textContent,
	).toContain("up");
});

it("surfaces a down class on fragment list and detail", async () => {
	vi.useFakeTimers();
	const { container } = await historical({
		symbol: "ETH/USD",
		class: "down",
		fields: {
			a_idx: 1,
			b_idx: 2,
			c_idx: 3,
			d_idx: 3,
			entry_idx: 1,
			exit_idx: 3,
			b_price: 3010,
			c_price: 3000,
		},
		trades: [
			{ seq: 1, price: 3010, at: ms(1) },
			{ seq: 2, price: 3005, at: ms(2) },
			{ seq: 3, price: 3000, at: ms(3) },
		],
	});

	expect(container.textContent).toContain("DOWNWARD EXCURSION");
	expect(container.textContent).toContain("[down]");
	expect(
		container.querySelector('[data-l="fragment-class-detail"]')?.textContent,
	).toContain("down");
});

it("lists the detected epochs and shows a failed stream instead of hiding it", async () => {
	vi.useFakeTimers();
	const { container } = await historical({
		symbol: "ETH/USD",
		class: "up",
		fields: {
			a_idx: 1,
			b_idx: 2,
			c_idx: 3,
			d_idx: 3,
			entry_idx: 1,
			exit_idx: 3,
		},
		trades: [{ seq: 1, price: 3000, at: ms(1) }],
	});

	expect(
		container.querySelector('[data-l="historical-runs-epoch"]')?.textContent,
	).toContain("9 ticks · 1 up / 0 down");

	await act(async () => {
		detectionsStore.setState((state) => ({
			...state,
			tape: state.tape && {
				...state.tape,
				status: "error",
				error: "tick 2 holds no trade at b_at",
			},
		}));
	});

	expect(
		container.querySelector('[data-l="historical-runs-error"]')?.textContent,
	).toContain("tick 2 holds no trade at b_at");
});

it("keeps a pinned historical fragment when live BTC hub updates arrive", async () => {
	vi.useFakeTimers();
	const { container } = await historical({
		symbol: "ETH/USD",
		class: "up",
		fields: {
			a_idx: 1,
			b_idx: 2,
			c_idx: 3,
			d_idx: 3,
			entry_idx: 1,
			exit_idx: 3,
			b_price: 3000,
			c_price: 3100,
		},
		trades: [
			{ seq: 1, price: 3000, at: ms(1) },
			{ seq: 2, price: 3050, at: ms(2) },
			{ seq: 3, price: 3100, at: ms(3) },
		],
	});

	expect(container.textContent).toContain("ETH/USD");
	expect(container.textContent).toContain("3 frames evaluated");
	expect(
		container.querySelector('[data-l="historical-run-active"]'),
	).toBeTruthy();

	const ring = new RingBuffer<Value>(8);
	await act(async () => {
		const frame = new Value({
			origin: "training",
			scope: "BTC/USD",
			tick: 99n,
			metrics: [
				{ name: "price", raw: 85000 },
				{ name: "stage_code", raw: 1 },
			],
		});
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
	const { container } = await historical({
		symbol: "LIGHTER/USD",
		class: "flat",
		fields: {
			a_idx: 271,
			b_idx: 275,
			c_idx: 277,
			d_idx: 277,
			entry_idx: 271,
			exit_idx: 277,
		},
		trades: [
			{ seq: 271, price: 3.091, at: ms(1718028000000) },
			{ seq: 273, price: 3.093, at: ms(1718028000090) },
			{ seq: 275, price: 3.095, at: ms(1718028000180) },
			{ seq: 276, price: 3.097, at: ms(1718028000270) },
			{ seq: 277, price: 3.098, at: ms(1718028000350) },
		],
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
	const { container } = await historical({
		symbol: "GNOT/USD",
		class: "flat",
		fields: {
			a_idx: 723,
			b_idx: 738,
			c_idx: 739,
			d_idx: 739,
			entry_idx: 723,
			exit_idx: 739,
		},
		trades: [
			{ seq: 723, price: 0.07381, at: ms(1718028000000) },
			{ seq: 738, price: 0.07382, at: ms(1718028000500) },
			{ seq: 739, price: 0.07382, at: ms(1718028000940) },
		],
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
